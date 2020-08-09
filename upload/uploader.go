package upload

import (
	"log"
	"os"
)

// MinMultipartUploadSize Size at which an upload is performed as a multipart upload rather than a single
const MinMultipartUploadSize int64 = 1024 * 1024 * 15

// Upload Uploads a file
// Files larger than 15MB will be uploaded as multipart upload
// All uploads are performed using presigned links from the database backend
func Upload(token string, uploadFilePath string, datasetVersionID string) error {
	uploadHandler, err := New(token)
	if err != nil {
		log.Println(err.Error())
		return err
	}

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

	if fileinfo.Size() > MinMultipartUploadSize {
		err = uploadHandler.UploadFileMultipart(file, datasetVersionID)
		if err != nil {
			log.Println(err.Error())
			return err
		}
	} else {
		err = uploadHandler.UploadFile(file, datasetVersionID)
		if err != nil {
			log.Println(err.Error())
			return err
		}
	}

	return nil
}
