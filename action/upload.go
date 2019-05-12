package action

import (
	"database/sql"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/gorilla/mux"
	"github.com/siddhartham/imageutil-thumbor/model"
	"github.com/siddhartham/imageutil-thumbor/util"
)

func UploadHandler(db *sql.DB, res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)

	util.LogInfo("UploadHandler", vars["uploadToken"])
	folder, status, err := checkTokenAndGetFolder(db, vars["uploadToken"], vars["fileName"])
	if err != nil {
		util.LogError("UploadHandler : checkTokenAndGetFolder", err.Error())
		res.WriteHeader(status)
		res.Write([]byte(err.Error()))
		return
	}

	req.ParseMultipartForm(32 << 20)
	file, _, err := req.FormFile("file")
	if err != nil {
		util.LogError("UploadHandler : get file", err.Error())
		res.WriteHeader(http.StatusBadRequest)
		res.Write([]byte(err.Error()))
		return
	}
	defer file.Close()

	path, err := uploadFile(vars["fileName"], file, folder)
	if err != nil {
		util.LogError("UploadHandler : uploadFile", err.Error())
		res.WriteHeader(http.StatusInternalServerError)
		res.Write([]byte(err.Error()))
		return
	}

	_, err = saveFileToDb(db, &folder, vars["fileName"], path)
	if err != nil {
		util.LogError("UploadHandler : saveFileToDb", err.Error())
		res.WriteHeader(http.StatusInternalServerError)
		res.Write([]byte(err.Error()))
		return
	}

	res.WriteHeader(http.StatusOK)
	res.Write([]byte("Uploaded!"))
}

func saveFileToDb(db *sql.DB, folder *model.Folder, name string, path string) (model.Folder, error) {
	file := model.Folder{
		UserID:    folder.UserID,
		ProjectID: folder.ProjectID,
		FolderID:  folder.ID,
		IsFile:    "1",
		Name:      name,
		Path:      path,
	}

	sqlStm := fmt.Sprintf("INSERT INTO folders (id, user_id, project_id, folder_id, is_file, name, path, created_at, updated_at) VALUES ( NULL, %s, %s, %s, %s, '%s', '%s', NOW(), NOW() )", file.UserID, file.ProjectID, file.FolderID, file.IsFile, file.Name, file.Path)
	_, err := db.Exec(sqlStm)
	if err != nil {
		return file, err
	}

	return file, nil
}

func checkTokenAndGetFolder(db *sql.DB, uploadToken string, fileName string) (model.Folder, int, error) {
	folder := model.Folder{}

	if fileName == "" || len(fileName) < 5 {
		return folder, http.StatusUnauthorized, errors.New("Invalid filename")
	}

	tokens := strings.Split(uploadToken, "_")
	if len(tokens) != 4 {
		return folder, http.StatusUnauthorized, errors.New("Invalid upload token")
	}

	dateValidity, err := strconv.ParseUint(tokens[3], 10, 32)
	if err != nil {
		return folder, http.StatusUnauthorized, err
	}
	currentTime := int32(time.Now().Unix())
	if currentTime > int32(dateValidity) {
		return folder, http.StatusUnauthorized, errors.New("Upload token is already expired")
	}

	sqlStm := fmt.Sprintf("SELECT id, user_id, project_id, name, path FROM folders where upload_token = '%s' and project_id=%s and user_id=%s", uploadToken, tokens[1], tokens[0])
	err = db.QueryRow(sqlStm).Scan(&folder.ID, &folder.UserID, &folder.ProjectID, &folder.Name, &folder.Path)
	if err != nil {
		return folder, http.StatusUnauthorized, err
	}

	file := model.Folder{}
	sqlStm = fmt.Sprintf("SELECT id FROM folders where project_id=%s and user_id=%s and folder_id=%s and name='%s'", folder.ProjectID, folder.UserID, folder.ID, fileName)
	err = db.QueryRow(sqlStm).Scan(&file.ID)
	if err == nil {
		return folder, http.StatusConflict, errors.New("File with same name already exists in this folder")
	}

	return folder, http.StatusOK, nil
}

func uploadFile(fileName string, f multipart.File, folder model.Folder) (string, error) {
	endpoint := os.Getenv("MEDIAENDPOINT")
	region := os.Getenv("MEDIAREGION")
	spaceKey := os.Getenv("SPACEKEY")
	spaceSecret := os.Getenv("SPACESECRET")
	doBucket := os.Getenv("BUCKETNAME")
	storageFolder := os.Getenv("MEDIASTORAGE")

	destPath := fmt.Sprintf("%s/%s/%s", storageFolder, folder.Path, fileName)

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
		return "", fmt.Errorf("failed to upload file, %v", err)
	}
	util.LogInfo("UploadHandler : ", aws.StringValue(&result.Location))
	return destPath, nil
}
