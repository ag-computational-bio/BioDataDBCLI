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

	"github.com/ag-computational-bio/BioDataDBModels/go/api"
	"github.com/ag-computational-bio/BioDataDBModels/go/commonmodels"
	"github.com/ag-computational-bio/BioDataDBModels/go/loadmodels"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Chunksize Default size of the uploaded chunks in multipart upload
const Chunksize = 1024 * 1024 * 5

// Handler Handles file upload
type Handler struct {
	LoadHandler        api.LoadServiceClient
	DefaultGRPCContext context.Context
	Token              string
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

	return &handler, nil
}

// UploadFile Uploads a single file with a single put command from a presigned url into object storage
func (handler *Handler) UploadFile(uploadFile *os.File, datasetVersion string) error {
	stat, err := uploadFile.Stat()
	if err != nil {
		log.Println(err.Error())
		return err
	}

	initUploadRequest := loadmodels.CreateUploadLinkRequest{
		DatasetVersionID: datasetVersion,
		ContentLen:       stat.Size(),
		Created:          timestamppb.Now(),
		Filename:         filepath.Base(uploadFile.Name()),
		Filetype:         "bin",
		Name:             filepath.Base(uploadFile.Name()),
		Version: &commonmodels.Version{
			Major:    1,
			Minor:    0,
			Patch:    0,
			Revision: 0,
			Stage:    commonmodels.Version_Stable,
		},
	}

	response, err := handler.LoadHandler.CreateUploadLink(handler.OutGoingContext(), &initUploadRequest)
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
func (handler *Handler) UploadFileMultipart(uploadFile *os.File, datasetVersion string) error {
	defer println()

	stat, err := uploadFile.Stat()
	if err != nil {
		log.Println(err.Error())
		return err
	}

	fileSize := stat.Size()

	initUploadRequest := loadmodels.InitMultipartUploadRequest{
		DatasetVersionID: datasetVersion,
		ContentLen:       stat.Size(),
		Created:          timestamppb.Now(),
		Filename:         filepath.Base(uploadFile.Name()),
		Filetype:         "bin",
		Name:             filepath.Base(uploadFile.Name()),
		Version: &commonmodels.Version{
			Major:    1,
			Minor:    0,
			Patch:    0,
			Revision: 0,
			Stage:    commonmodels.Version_Stable,
		},
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

		uploadedParts = append(uploadedParts, &loadmodels.CompletedUploadParts{
			Etag:       putResponse.Header["Etag"][0],
			Partnumber: i,
		})

		percentage := (float64(uploadedBytes) / float64(fileSize)) * 100.0

		print(fmt.Sprintf("\rPercentage of bytes uploaded: %.2f%%", percentage))
	}

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
	return metadata.AppendToOutgoingContext(handler.DefaultGRPCContext, "user_api_token", handler.Token)
}
