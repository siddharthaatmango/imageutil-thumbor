package action

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/gorilla/mux"
	"github.com/siddhartham/imageutil-thumbor/util"
)

func UploadHandler(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)

	util.LogInfo("UploadHandler", vars["token"])

	req.ParseMultipartForm(32 << 20)
	file, handler, err := req.FormFile("file")
	if err != nil {
		util.LogError("UploadHandler : get file", err.Error())
	}
	defer file.Close()

	err = uploadFile(handler.Filename, file)
	if err != nil {
		util.LogError("UploadHandler : uploadFile", err.Error())
	}

	res.WriteHeader(http.StatusOK)
	fmt.Fprintf(res, "UploadHandler : %v\n", handler.Filename)
}

func uploadFile(fileName string, f multipart.File) error {
	endpoint := os.Getenv("ENDPOINT")
	region := os.Getenv("REGION")
	spaceKey := os.Getenv("SPACEKEY")
	spaceSecret := os.Getenv("SPACESECRET")
	doBucket := os.Getenv("BUCKETNAME")
	storageFolder := os.Getenv("STORAGE_FOLDER")

	destPath := fmt.Sprintf("%s/%s", storageFolder, fileName)

	sess := session.Must(session.NewSession(&aws.Config{
		Endpoint: &endpoint,
		Region:   &region,
		Credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     spaceKey,
			SecretAccessKey: spaceSecret,
		}),
	}))

	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	// Upload the file to do
	result, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(doBucket),
		Key:    aws.String(destPath),
		Body:   f,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file, %v", err)
	}
	util.LogInfo("UploadHandler : ", aws.StringValue(&result.Location))
	return nil
}
