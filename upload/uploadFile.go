package upload

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ag-computational-bio/BioDataDBModels/go/datasetentrymodels"

	"github.com/ag-computational-bio/BioDataDBModels/go/commonmodels"

	"github.com/ag-computational-bio/BioDataDBModels/go/datasetapimodels"

	"github.com/ag-computational-bio/BioDataDBModels/go/api"
	"github.com/ag-computational-bio/BioDataDBModels/go/loadmodels"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Chunksize Default size of the uploaded chunks in multipart upload
const Chunksize = 1024 * 1024 * 10

// Handler Handles file upload
type Handler struct {
	LoadHandler          api.LoadServiceClient
	DatasetObjectHandler api.ObjectsServiceClient
	DatasetHandler       api.DatasetServiceClient
	DefaultGRPCContext   context.Context
	Token                string
}

// New Creates a new upload handler
func New(token string) (*Handler, error) {
	handler := Handler{}

	handler.Token = token

	ctx := context.Background()
	handler.DefaultGRPCContext = ctx

	var tlsConf tls.Config

	credentials := credentials.NewTLS(&tlsConf)

	host := viper.GetString("Config.GRPCEndpoint.Host")
	if host == "" {
		err := fmt.Errorf("Endpoints datasethandler host needs to be set")
		log.Println(err.Error())
		return nil, err
	}

	port := viper.GetInt("Config.GRPCEndpoint.Port")
	if port == 0 {
		err := fmt.Errorf("Endpoints datasethandler port needs to be set")
		log.Println(err.Error())
		return nil, err
	}

	dialOptions := grpc.WithTransportCredentials(credentials)
	if host == "localhost" {
		dialOptions = grpc.WithInsecure()
	}

	conn, err := grpc.Dial(fmt.Sprintf("%v:%v", host, port), dialOptions)
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}

	loadClient := api.NewLoadServiceClient(conn)
	handler.LoadHandler = loadClient

	objectClient := api.NewObjectsServiceClient(conn)
	handler.DatasetObjectHandler = objectClient

	datasetClient := api.NewDatasetServiceClient(conn)
	handler.DatasetHandler = datasetClient

	return &handler, nil
}

// Upload Uploads a file
// Files larger than 10MB will be uploaded as multipart upload
// All uploads are performed using presigned links from the database backend
func (handler *Handler) Upload(token string, uploadFilePaths []string, datasetID string, datasetVersionID string) error {

	request := &datasetapimodels.CreateDatasetObjectGroupRequest{
		DatasetID: datasetID,
		Name:      "Test",
		Version: &commonmodels.Version{
			Major:    0,
			Minor:    2,
			Patch:    0,
			Revision: 0,
			Stage:    commonmodels.Version_Stable,
		},
		DatasetVersionID: []string{datasetVersionID},
	}

	objectGroup, err := handler.DatasetObjectHandler.CreateDatsetObjectGroup(handler.OutGoingContext(), request)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	uploadHandler, err := New(token)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	for _, uploadFilePath := range uploadFilePaths {
		file, err := os.Open(uploadFilePath)
		if err != nil {
			log.Println(err.Error())
			return err
		}

		fileinfo, err := file.Stat()
		if err != nil {
			log.Println(err.Error())
			return err
		}

		if fileinfo.Size() > int64(MinMultipartUploadSize) {
			err = uploadHandler.UploadFileMultipart(file, objectGroup.GetID())
			if err != nil {
				log.Println(err.Error())
				return err
			}
		} else {
			err = uploadHandler.UploadFile(file, objectGroup.GetID())
			if err != nil {
				log.Println(err.Error())
				return err
			}
		}
	}

	statusUpdate := datasetapimodels.StatusUpdate{
		ID:     datasetVersionID,
		Status: datasetentrymodels.Status_Available,
	}

	_, err = handler.DatasetHandler.UpdateDatasetVersionStatus(handler.OutGoingContext(), &statusUpdate)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	updateCurrentDatasetVersion := datasetapimodels.UpdateCurrentDatasetVersionRequest{
		ID:             datasetID,
		TargetResource: commonmodels.Resource_DatasetVersion,
		UpdateTargetID: datasetVersionID,
		UpdateStage:    commonmodels.Stage_Stable,
	}

	_, err = handler.DatasetHandler.UpdateCurrentDatasetVersion(handler.OutGoingContext(), &updateCurrentDatasetVersion)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	return nil
}

// UploadFile Uploads a single file with a single put command from a presigned url into object storage
func (handler *Handler) UploadFile(uploadFile *os.File, datasetObjectGroupID string) error {
	stat, err := uploadFile.Stat()
	if err != nil {
		log.Println(err.Error())
		return err
	}

	ext := filepath.Ext(stat.Name())

	initUploadRequest := loadmodels.CreateUploadLinkRequest{
		CreateDatasetObjectRequest: &loadmodels.CreateDatasetObjectRequest{
			ContentLen: stat.Size(),
			Created:    timestamppb.Now(),
			Filename:   stat.Name(),
			Filetype:   ext,
		},
		DatasetObjectGroupID: datasetObjectGroupID,
	}

	response, err := handler.LoadHandler.GetUploadLink(handler.OutGoingContext(), &initUploadRequest)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	data, err := ioutil.ReadAll(uploadFile)

	dataReader := bytes.NewReader(data)

	putRequest, err := http.NewRequest(http.MethodPut, response.GetLink(), dataReader)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	putResponse, err := http.DefaultClient.Do(putRequest)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	if putResponse.StatusCode != http.StatusOK {
		log.Println(putResponse.Status)
		return fmt.Errorf("Error while uploading data")
	}

	return nil
}

// UploadFileMultipart Uploads the file
func (handler *Handler) UploadFileMultipart(uploadFile *os.File, datasetObjectGroupID string) error {
	defer println()

	stat, err := uploadFile.Stat()
	if err != nil {
		log.Println(err.Error())
		return err
	}

	ext := filepath.Ext(stat.Name())

	fileSize := stat.Size()

	initUploadRequest := loadmodels.InitMultipartUploadRequest{
		CreateDatasetObjectRequest: &loadmodels.CreateDatasetObjectRequest{
			ContentLen: stat.Size(),
			Created:    timestamppb.Now(),
			Filename:   stat.Name(),
			Filetype:   ext,
		},
		DatasetObjectGroupID: datasetObjectGroupID,
	}

	initMultipartUploadResponse, err := handler.LoadHandler.InitMultipartUpload(handler.OutGoingContext(), &initUploadRequest)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	reader := bufio.NewReader(uploadFile)
	var i int64
	var uploadedParts []*loadmodels.CompletedUploadParts

	var uploadedBytes int64
	percentage := 0.0 / float64(fileSize) * 100.0

	print(fmt.Sprintf("\rPercentage of bytes uploaded: %.2f%%", percentage))

	for {
		i++
		data := make([]byte, Chunksize)
		n, err := reader.Read(data)
		if err != nil && err != io.EOF {
			log.Println(err.Error())
			return err
		}

		if err == io.EOF {
			break
		}

		if n < Chunksize {
			data = data[:n]
		}

		request := loadmodels.GetMultipartUploadLinkPartRequest{
			DatasetObjectID: initMultipartUploadResponse.GetDatasetObjectID(),
			UploadPart:      i,
			ContentLen:      int64(n),
		}

		response, err := handler.LoadHandler.GetMultipartUploadLinkPart(handler.OutGoingContext(), &request)
		if err != nil {
			log.Println(err.Error())
			return err
		}

		uploadLink := response.GetUploadLink()

		dataReader := bytes.NewReader(data)

		putRequest, err := http.NewRequest(http.MethodPut, uploadLink, dataReader)
		if err != nil {
			log.Println(err.Error())
			return err
		}

		putResponse, err := http.DefaultClient.Do(putRequest)
		if err != nil {
			log.Println(err.Error())
			return err
		}

		uploadedBytes = uploadedBytes + int64(n)

		if putResponse.StatusCode != http.StatusOK {
			log.Println(putResponse.Status)
			return fmt.Errorf("Error while uploading data")
		}

		correctEtag := strings.ReplaceAll(putResponse.Header["Etag"][0], "\"", "")

		uploadedParts = append(uploadedParts, &loadmodels.CompletedUploadParts{
			Etag:       correctEtag,
			Partnumber: i,
		})

		percentage := (float64(uploadedBytes) / float64(fileSize)) * 100.0

		print(fmt.Sprintf("\rPercentage of bytes uploaded: %.2f%%", percentage))
	}
	println()

	_, err = handler.LoadHandler.FinishMultipartUpload(handler.OutGoingContext(), &loadmodels.FinishMultipartUploadRequest{
		CompletedUploadParts: uploadedParts,
		DatasetObjectID:      initMultipartUploadResponse.GetDatasetObjectID(),
	})
	if err != nil {
		log.Println(err.Error())
		return err
	}

	return nil
}

// OutGoingContext Creates the required outgoing context for a call
func (handler *Handler) OutGoingContext() context.Context {
	mdMap := make(map[string]string)
	mdMap["UserAPIToken"] = handler.Token
	tokenMetadata := metadata.New(mdMap)

	outgoingContext := metadata.NewOutgoingContext(context.TODO(), tokenMetadata)
	return outgoingContext
}
