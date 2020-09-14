package upload

import (
	"log"
)

// MinMultipartUploadSize Size at which an upload is performed as a multipart upload rather than a single
const MinMultipartUploadSize int = 1024 * 1024 * 15

// Upload Uploads a file
// Files larger than 15MB will be uploaded as multipart upload
// All uploads are performed using presigned links from the database backend
func Upload(token string, uploadFilePaths []string, datasetID string, datasetVersionID string) error {
	uploadHandler, err := New(token)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	uploadHandler.Upload(token, uploadFilePaths, datasetID, datasetVersionID)

	return nil
}
