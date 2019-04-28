package action

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"github.com/siddhartham/imageutil-thumbor/model"
	"github.com/siddhartham/imageutil-thumbor/util"
)

func SaveImageUrl(db *sql.DB, image model.Image, analytic model.Analytic) {
	sqlStm := fmt.Sprintf("INSERT INTO images VALUES ( NULL, %s, %s, '%s', '%s', '%s', '%s', '%s', '%s', 0, NOW(), NOW(), 'imagetransform.io')", image.UserID, image.ProjectID, image.Key, image.Origin, image.OriginPath, image.Transformation, image.IsSmart, image.CdnPath)
	insert, err := db.Exec(sqlStm)
	if err != nil {
		util.LogError("saveImageUrl : INSERT", err.Error())
	} else {
		id, _ := insert.LastInsertId()
		analytic.ImageID = strconv.FormatInt(id, 10)
		SaveAnalytic(db, image, analytic, 1, 1, 0)
	}
}

func UpdateImageFileSize(db *sql.DB, image model.Image) {
	sqlStm, err := db.Prepare("UPDATE images SET file_size=?  WHERE id=?")
	if err != nil {
		util.LogError("updateImageFileSize : UPDATE : Prepare", err.Error())
	}
	_, err = sqlStm.Exec(image.FileSize, image.ID)
	if err != nil {
		util.LogError("updateImageFileSize : UPDATE: Exec", err.Error())
	}
}

func SaveAnalytic(db *sql.DB, image model.Image, analytic model.Analytic, incrUniq int64, incrTotal int64, incrBytes int64) {
	sqlStm := fmt.Sprintf("SELECT id, uniq_request, total_request, total_bytes FROM analytics where project_id = %s and DATE(created_at) = CURDATE()", analytic.ProjectID)
	err := db.QueryRow(sqlStm).Scan(&analytic.ID, &analytic.UniqRequest, &analytic.TotalRequest, &analytic.TotalBytes)
	if err != nil {
		util.LogWarning("saveAnalytic : SELECT", err.Error())
		analytic.UniqRequest = 1
		analytic.TotalRequest = 1
		analytic.TotalBytes = incrBytes
		sqlStm = fmt.Sprintf("INSERT INTO analytics VALUES ( NULL, %s, %s, '%d', '%d', '%d', '%s', NOW(), NOW() )", analytic.UserID, analytic.ProjectID, analytic.UniqRequest, analytic.TotalRequest, analytic.TotalBytes, analytic.ImageID)
		_, err := db.Exec(sqlStm)
		if err != nil {
			util.LogError("saveAnalytic : INSERT", err.Error())
		}
	} else {
		analytic.UniqRequest = analytic.UniqRequest + incrUniq
		analytic.TotalRequest = analytic.TotalRequest + incrTotal
		analytic.TotalBytes = analytic.TotalBytes + incrBytes
		if image.FileSize == 0 {
			resp, err := http.Get(image.ImgURL)
			if err == nil {
				if resp.StatusCode == 200 {
					if err == nil {
						image.FileSize = resp.ContentLength
						analytic.TotalBytes = analytic.TotalBytes + image.FileSize
						UpdateImageFileSize(db, image)
					}
				}
			} else {
				util.LogError("saveAnalytic : GetBytes", err.Error())
			}
		}
		sqlStm, err := db.Prepare("UPDATE analytics SET uniq_request=?, total_request=?, total_bytes=?,last_image_id=?  WHERE id=?")
		if err != nil {
			util.LogError("saveAnalytic : UPDATE : Prepare", err.Error())
		}
		_, err = sqlStm.Exec(analytic.UniqRequest, analytic.TotalRequest, analytic.TotalBytes, analytic.ImageID, analytic.ID)
		if err != nil {
			util.LogError("saveAnalytic : UPDATE: Exec", err.Error())
		}
	}
}
