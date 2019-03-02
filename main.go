package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

var cyan = color.New(color.FgCyan)
var boldCyan = cyan.Add(color.Bold)

var yellow = color.New(color.FgHiYellow)
var boldYellow = yellow.Add(color.Bold)

var red = color.New(color.FgRed)
var boldRed = red.Add(color.Bold)

var green = color.New(color.FgGreen)
var boldGreen = green.Add(color.Bold)

type serverConf struct {
	host                string
	port                string
	thumborHost         string
	thumborSecret       string
	mysqlServerHost     string
	mysqlServerPort     string
	mysqlServerUsername string
	mysqlServerPassword string
	mysqlServerDatabase string
	cdnOrigin           string
	bucketName          string
	resultStorage       string
}

type override struct {
	Match   string
	Replace string
}

type config struct {
	Path            string
	Host            string
	IsSmart         bool
	Secret          string
	MysqlServerConn *sql.DB
	cdnOrigin       string
	bucketName      string
	resultStorage   string
}

type Project struct {
	ID       string
	userID   string
	uuid     string
	fqdn     string
	protocol string
	basePath string
}

type Image struct {
	ID             string
	userID         string
	projectID      string
	key            string
	origin         string
	originPath     string
	transformation string
	isSmart        string
	cdnPath        string
	fileSize       int64
	imgURL         string
}

type Analytic struct {
	ID           string
	userID       string
	projectID    string
	imageID      string
	uniqRequest  int64
	totalRequest int64
	totalBytes   int64
}

func logWarning(str1 string, str2 string) {
	yellow.Println("Warning : ", str1)
	yellow.Println(str2)
}

func logSuccess(str1 string, str2 string) {
	green.Println("Success : ", str1)
	green.Println(str2)
}

func logInfo(str1 string, str2 string) {
	cyan.Println("Info : ", str1)
	cyan.Println(str2)
}

func logError(str1 string, str2 string) {
	red.Println("Error : ", str1)
	red.Println(str2)
}

func generateProxy(conf config) http.Handler {
	proxy := &httputil.ReverseProxy{Director: func(req *http.Request) {
		vars := mux.Vars(req)

		// to be fetched from db
		projectID := vars["project_id"]
		var project Project
		var image Image
		var analytic Analytic
		// Execute the query
		sqlStm := fmt.Sprintf("SELECT id, user_id, uuid, fqdn, protocol, base_path FROM projects where uuid = '%s' and is_active=1", projectID)
		err := conf.MysqlServerConn.QueryRow(sqlStm).Scan(&project.ID, &project.userID, &project.uuid, &project.fqdn, &project.protocol, &project.basePath)
		if err != nil {
			logError("generateProxy : SELECT", err.Error())
			return
		}
		projectImageOrigin := fmt.Sprintf("%s://%s", project.protocol, project.fqdn)
		if project.basePath != "" {
			projectImageOrigin = fmt.Sprintf("%s://%s/%s", project.protocol, project.fqdn, project.basePath)
		}

		image.userID = project.userID
		image.projectID = project.ID
		image.origin = projectImageOrigin
		image.originPath = vars["image"]
		image.transformation = vars["transformation"]
		image.isSmart = "0"
		if conf.IsSmart {
			image.isSmart = "1"
		}

		analytic.userID = project.userID
		analytic.projectID = project.ID

		sqlStm = fmt.Sprintf("SELECT id, cdn_path, file_size FROM images where project_id = %s and origin_path = '%s' and transformation = '%s' and is_smart = %s", image.projectID, image.originPath, image.transformation, image.isSmart)
		err = conf.MysqlServerConn.QueryRow(sqlStm).Scan(&image.ID, &image.cdnPath, &image.fileSize)
		if err != nil {
			logWarning("generateProxy : SELECT", err.Error())
		}

		finalScheme := project.protocol
		finalHost := conf.cdnOrigin
		finalPath := strings.Replace(image.cdnPath, fmt.Sprintf("%s/", conf.resultStorage), "", 1)
		image.imgURL = fmt.Sprintf("%s://%s%s", finalScheme, finalHost, finalPath)
		if finalPath == "" {
			finalScheme = "http" //thumbor is internal
			finalHost = conf.Host
			finalPath = getThumborUrl(conf, projectImageOrigin, image, analytic)
		} else {
			req.Host = conf.cdnOrigin
			analytic.imageID = image.ID
			go saveAnalytic(conf.MysqlServerConn, image, analytic, 0, 1, 0)
		}
		//rewrite url
		req.URL = &url.URL{
			Scheme:  finalScheme,
			Host:    finalHost,
			Path:    finalPath,
			RawPath: finalPath,
		}
		logInfo("generateProxy : FinalURL", fmt.Sprintf("%s", req.URL))

		//set headers
		req.Header.Add("X-Forwarded-Host", req.Host)
		req.Header.Add("X-Origin-Host", finalHost)

		logInfo("generateProxy : X-Forwarded-Host", req.Host)
		logInfo("generateProxy : X-Origin-Host", finalHost)

	}, Transport: &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).Dial,
	}}

	return proxy
}

func getThumborUrl(conf config, projectImageOrigin string, image Image, analytic Analytic) string {
	//attach origin of image
	imageURL := fmt.Sprintf("%s/%s", projectImageOrigin, image.originPath)

	//set the size
	se := regexp.MustCompile(`s:(\d*)x(\d*)`)
	size := se.FindAllStringSubmatch(image.transformation, -1)[0]
	transformationStr := fmt.Sprintf("%sx%s", size[1], size[2])

	//set the policy
	pe := regexp.MustCompile(`p:(crop|fit)-?(top|middle|bottom)?-?(left|center|right)?`)
	policy := pe.FindAllStringSubmatch(image.transformation, -1)
	if len(policy) > 0 {
		switch policy[0][1] {
		case "fit":
			transformationStr = fmt.Sprintf("fit-in/%s", transformationStr)
		default:
			transformationStr = fmt.Sprintf("trim/%s", transformationStr)
		}
		HALIGN := "left"
		VALIGN := "top"
		if policy[0][2] != "" {
			VALIGN = policy[0][2]
		}
		if policy[0][3] != "" {
			HALIGN = policy[0][3]
		}
		transformationStr = fmt.Sprintf("%s/%s/%s", transformationStr, HALIGN, VALIGN)
	}

	//set smart detect
	if conf.IsSmart {
		transformationStr = fmt.Sprintf("%s/smart", transformationStr)
	}

	//filters
	filters := ""
	//set the quality
	qe := regexp.MustCompile(`q:(\d*)`)
	quality := qe.FindAllStringSubmatch(image.transformation, -1)
	if len(quality) > 0 {
		filters = fmt.Sprintf("%s:quality(%s)", filters, quality[0][1])
	}
	//set the format
	fe := regexp.MustCompile(`f:(webp|jpeg|gif|png)`)
	format := fe.FindAllStringSubmatch(image.transformation, -1)
	if len(format) > 0 {
		filters = fmt.Sprintf("%s:format(%s)", filters, format[0][1])
	}
	//set other effects
	ee := regexp.MustCompile(`e:(brightness|contrast|rgb|round_corner|noise|watermark)\(?([^\)]*)?\)`)
	effects := ee.FindAllStringSubmatch(image.transformation, -1)
	if len(effects) > 0 && len(effects[0]) == 2 {
		filters = fmt.Sprintf("%s:%s()", filters, effects[0][1])
	} else if len(effects) > 0 && len(effects[0]) == 3 {
		filters = fmt.Sprintf("%s:%s(%s)", filters, effects[0][1], effects[0][2])
	}
	//set the filters
	if filters != "" {
		transformationStr = fmt.Sprintf("%s/filters%s", transformationStr, filters)
	}

	//thumbor path
	thumborPath := fmt.Sprintf("%s/%s", transformationStr, imageURL)

	//calculate signature
	hash := hmac.New(sha1.New, []byte(conf.Secret))
	hash.Write([]byte(thumborPath))
	message := hash.Sum(nil)
	signature := base64.URLEncoding.EncodeToString(message)
	image.key = signature

	//final path
	finalPath := fmt.Sprintf("/%s/%s", signature, thumborPath)

	//cdn path
	_, fileName := path.Split(image.originPath)
	reg, _ := regexp.Compile("[^a-zA-Z0-9]+")
	processedKey := reg.ReplaceAllString(image.key, "_")

	image.cdnPath = fmt.Sprintf("/%s/%s/%s", conf.resultStorage, processedKey, fileName)
	go saveImageUrl(conf.MysqlServerConn, image, analytic)

	return finalPath
}

func saveImageUrl(db *sql.DB, image Image, analytic Analytic) {
	sqlStm := fmt.Sprintf("INSERT INTO images VALUES ( NULL, %s, %s, '%s', '%s', '%s', '%s', '%s', '%s', 0, NOW(), NOW(), 'imagetransform.io')", image.userID, image.projectID, image.key, image.origin, image.originPath, image.transformation, image.isSmart, image.cdnPath)
	insert, err := db.Exec(sqlStm)
	if err != nil {
		logError("saveImageUrl : INSERT", err.Error())
	} else {
		id, _ := insert.LastInsertId()
		analytic.imageID = strconv.FormatInt(id, 10)
		saveAnalytic(db, image, analytic, 1, 1, 0)
	}
}

func saveAnalytic(db *sql.DB, image Image, analytic Analytic, incrUniq int64, incrTotal int64, incrBytes int64) {
	sqlStm := fmt.Sprintf("SELECT id, uniq_request, total_request, total_bytes FROM analytics where project_id = %s and DATE(created_at) = CURDATE()", analytic.projectID)
	err := db.QueryRow(sqlStm).Scan(&analytic.ID, &analytic.uniqRequest, &analytic.totalRequest, &analytic.totalBytes)
	if err != nil {
		logWarning("saveAnalytic : SELECT", err.Error())
		analytic.uniqRequest = 1
		analytic.totalRequest = 1
		analytic.totalBytes = incrBytes
		sqlStm = fmt.Sprintf("INSERT INTO analytics VALUES ( NULL, %s, %s, '%d', '%d', '%d', '%s', NOW(), NOW() )", analytic.userID, analytic.projectID, analytic.uniqRequest, analytic.totalRequest, analytic.totalBytes, analytic.imageID)
		_, err := db.Exec(sqlStm)
		if err != nil {
			logError("saveAnalytic : INSERT", err.Error())
		}
	} else {
		analytic.uniqRequest = analytic.uniqRequest + incrUniq
		analytic.totalRequest = analytic.totalRequest + incrTotal
		analytic.totalBytes = analytic.totalBytes + incrBytes
		if image.fileSize == 0 {
			resp, err := http.Get(image.imgURL)
			if err == nil {
				if resp.StatusCode == 200 {
					if err == nil {
						image.fileSize = resp.ContentLength
						analytic.totalBytes = analytic.totalBytes + image.fileSize
						updateImageFileSize(db, image)
					}
				}
			} else {
				logError("saveAnalytic : GetBytes", err.Error())
			}
		}
		sqlStm, err := db.Prepare("UPDATE analytics SET uniq_request=?, total_request=?, total_bytes=?,last_image_id=?  WHERE id=?")
		if err != nil {
			logError("saveAnalytic : UPDATE : Prepare", err.Error())
		}
		_, err = sqlStm.Exec(analytic.uniqRequest, analytic.totalRequest, analytic.totalBytes, analytic.imageID, analytic.ID)
		if err != nil {
			logError("saveAnalytic : UPDATE: Exec", err.Error())
		}
	}
}

func updateImageFileSize(db *sql.DB, image Image) {
	sqlStm, err := db.Prepare("UPDATE images SET file_size=?  WHERE id=?")
	if err != nil {
		logError("updateImageFileSize : UPDATE : Prepare", err.Error())
	}
	_, err = sqlStm.Exec(image.fileSize, image.ID)
	if err != nil {
		logError("updateImageFileSize : UPDATE: Exec", err.Error())
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	args := os.Args[1:]

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	port := os.Getenv("PORT")
	thumborHost := os.Getenv("THUMBORHOST")
	if len(args) > 0 {
		p, err := strconv.Atoi(args[0])
		if err == nil {
			port = fmt.Sprintf("%s%d", ":900", p)
			thumborHost = fmt.Sprintf("%s%d", "127.0.0.1:800", p)
		}
	}

	//General config, should load from env in prod
	sc := &serverConf{
		host:                os.Getenv("HOST"),
		port:                port,
		thumborHost:         thumborHost,
		thumborSecret:       os.Getenv("THUMBORSECRET"),
		mysqlServerHost:     os.Getenv("MYSQLSERVERHOST"),
		mysqlServerPort:     os.Getenv("MYSQLSERVERPORT"),
		mysqlServerUsername: os.Getenv("MYSQLSERVERUSERNAME"),
		mysqlServerPassword: os.Getenv("MYSQLSERVERPASSWORD"),
		mysqlServerDatabase: os.Getenv("MYSQLSERVERDATABASE"),
		cdnOrigin:           os.Getenv("CDNORIGIN"), //"cdn.imageutil.io",
		bucketName:          os.Getenv("BUCKETNAME"),
		resultStorage:       os.Getenv("RESULTSTORAGE"),
	}
	//Mysql connection
	mysqlConnStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", sc.mysqlServerUsername, sc.mysqlServerPassword, sc.mysqlServerHost, sc.mysqlServerPort, sc.mysqlServerDatabase)
	db, err := sql.Open("mysql", mysqlConnStr)
	if err != nil {
		panic(err.Error())
	}
	defer db.Close()

	//Reverse proxy router
	r := mux.NewRouter()
	configuration := []config{
		config{
			Path:            "/{project_id}/{transformation}/smart/{image:.*}",
			Host:            sc.thumborHost,
			IsSmart:         true,
			Secret:          sc.thumborSecret,
			MysqlServerConn: db,
			cdnOrigin:       sc.cdnOrigin,
			bucketName:      sc.bucketName,
			resultStorage:   sc.resultStorage,
		},
		config{
			Path:            "/{project_id}/{transformation}/{image:.*}",
			Host:            sc.thumborHost,
			IsSmart:         false,
			Secret:          sc.thumborSecret,
			MysqlServerConn: db,
			cdnOrigin:       sc.cdnOrigin,
			bucketName:      sc.bucketName,
			resultStorage:   sc.resultStorage,
		},
	}
	for _, conf := range configuration {
		proxy := generateProxy(conf)
		r.HandleFunc(conf.Path, func(w http.ResponseWriter, r *http.Request) {
			proxy.ServeHTTP(w, r)
		})
	}

	//Start server
	boldGreen.Println("Starting imageutil server on port", sc.port)
	log.Fatal(http.ListenAndServe(sc.port, r))
}
