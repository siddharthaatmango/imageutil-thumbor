package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/siddhartham/imageutil-thumbor/action"
	"github.com/siddhartham/imageutil-thumbor/model"
	"github.com/siddhartham/imageutil-thumbor/thumbor"
	"github.com/siddhartham/imageutil-thumbor/util"
)

func generateProxy(conf model.Config) http.Handler {
	proxy := &httputil.ReverseProxy{Director: func(req *http.Request) {
		vars := mux.Vars(req)

		// to be fetched from db
		projectID := vars["project_id"]

		// models
		var project model.Project
		var image model.Image
		var analytic model.Analytic

		// get projects
		projectImageOrigin, err := action.GetProject(conf.MysqlServerConn, projectID, &project)
		if err != nil {
			util.LogError("generateProxy : GetProject : SELECT", err.Error())
			return
		}

		// get image
		err = action.GetImage(conf.MysqlServerConn, conf.IsSmart, projectImageOrigin, vars["image"], vars["transformation"], &project, &image, &analytic)
		if err != nil {
			util.LogWarning("generateProxy : GetImage : SELECT", err.Error())
		}

		// get or generate thumbor
		finalScheme := project.Protocol
		finalHost := conf.CdnOrigin
		finalPath := strings.Replace(image.CdnPath, fmt.Sprintf("%s/", conf.ResultStorage), "", 1)
		image.ImgURL = fmt.Sprintf("%s://%s%s", finalScheme, finalHost, finalPath)
		if finalPath == "" {
			finalScheme = "http" //thumbor is internal
			finalHost = conf.Host
			finalPath = thumbor.GetThumborUrl(conf, projectImageOrigin, image, analytic)
		} else {
			req.Host = conf.CdnOrigin
			analytic.ImageID = image.ID
			go action.SaveAnalytic(conf.MysqlServerConn, image, analytic, 0, 1, 0)
		}

		//rewrite url
		req.URL = &url.URL{
			Scheme:  finalScheme,
			Host:    finalHost,
			Path:    finalPath,
			RawPath: finalPath,
		}
		util.LogInfo("generateProxy : FinalURL", fmt.Sprintf("%s", req.URL))

		//set headers
		req.Header.Add("X-Forwarded-Host", req.Host)
		req.Header.Add("X-Origin-Host", finalHost)

		util.LogInfo("generateProxy : X-Forwarded-Host", req.Host)
		util.LogInfo("generateProxy : X-Origin-Host", finalHost)

	}, Transport: &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).Dial,
	}}

	return proxy
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
	sc := &model.ServerConf{
		Host:                os.Getenv("HOST"),
		Port:                port,
		ThumborHost:         thumborHost,
		ThumborSecret:       os.Getenv("THUMBORSECRET"),
		MysqlServerHost:     os.Getenv("MYSQLSERVERHOST"),
		MysqlServerPort:     os.Getenv("MYSQLSERVERPORT"),
		MysqlServerUsername: os.Getenv("MYSQLSERVERUSERNAME"),
		MysqlServerPassword: os.Getenv("MYSQLSERVERPASSWORD"),
		MysqlServerDatabase: os.Getenv("MYSQLSERVERDATABASE"),
		CdnOrigin:           os.Getenv("CDNORIGIN"), //"cdn.imageutil.io",
		BucketName:          os.Getenv("BUCKETNAME"),
		ResultStorage:       os.Getenv("RESULTSTORAGE"),
	}
	//Mysql connection
	mysqlConnStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", sc.MysqlServerUsername, sc.MysqlServerPassword, sc.MysqlServerHost, sc.MysqlServerPort, sc.MysqlServerDatabase)
	db, err := sql.Open("mysql", mysqlConnStr)
	if err != nil {
		panic(err.Error())
	}
	defer db.Close()

	//Reverse proxy router
	r := mux.NewRouter()
	configuration := []model.Config{
		model.Config{
			Path:            "/{project_id}/{transformation}/smart/{image:.*}",
			Host:            sc.ThumborHost,
			IsSmart:         true,
			Secret:          sc.ThumborSecret,
			MysqlServerConn: db,
			CdnOrigin:       sc.CdnOrigin,
			BucketName:      sc.BucketName,
			ResultStorage:   sc.ResultStorage,
		},
		model.Config{
			Path:            "/{project_id}/{transformation}/{image:.*}",
			Host:            sc.ThumborHost,
			IsSmart:         false,
			Secret:          sc.ThumborSecret,
			MysqlServerConn: db,
			CdnOrigin:       sc.CdnOrigin,
			BucketName:      sc.BucketName,
			ResultStorage:   sc.ResultStorage,
		},
	}
	for _, conf := range configuration {
		proxy := generateProxy(conf)
		r.HandleFunc(conf.Path, func(w http.ResponseWriter, r *http.Request) {
			proxy.ServeHTTP(w, r)
		})
	}

	//Start server
	util.LogInfo("Starting imageutil server on port", sc.Port)
	log.Fatal(http.ListenAndServe(sc.Port, r))
}
