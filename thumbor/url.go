package thumbor

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"path"
	"regexp"

	"github.com/siddhartham/imageutil-thumbor/action"
	"github.com/siddhartham/imageutil-thumbor/model"
)

func GetThumborUrl(conf model.Config, projectImageOrigin string, image model.Image, analytic model.Analytic) string {
	//attach origin of image
	imageURL := image.OriginPath
	if conf.IsMedia == false {
		imageURL = fmt.Sprintf("%s/%s", projectImageOrigin, image.OriginPath)
	}

	//set the size
	se := regexp.MustCompile(`s:(\d*)x(\d*)`)
	size := se.FindAllStringSubmatch(image.Transformation, -1)[0]
	transformationStr := fmt.Sprintf("%sx%s", size[1], size[2])

	//set the policy
	pe := regexp.MustCompile(`p:(crop|fit)-?(top|middle|bottom)?-?(left|center|right)?`)
	policy := pe.FindAllStringSubmatch(image.Transformation, -1)
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
	quality := qe.FindAllStringSubmatch(image.Transformation, -1)
	if len(quality) > 0 {
		filters = fmt.Sprintf("%s:quality(%s)", filters, quality[0][1])
	}
	//set the format
	fe := regexp.MustCompile(`f:(webp|jpeg|gif|png)`)
	format := fe.FindAllStringSubmatch(image.Transformation, -1)
	if len(format) > 0 {
		filters = fmt.Sprintf("%s:format(%s)", filters, format[0][1])
	}
	//set other effects
	ee := regexp.MustCompile(`e:(brightness|contrast|rgb|round_corner|noise|watermark)\(?([^\)]*)?\)`)
	effects := ee.FindAllStringSubmatch(image.Transformation, -1)
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
	image.Key = signature

	//final path
	finalPath := fmt.Sprintf("/%s/%s", signature, thumborPath)

	//cdn path
	_, fileName := path.Split(image.OriginPath)
	reg, _ := regexp.Compile("[^a-zA-Z0-9]+")
	processedKey := reg.ReplaceAllString(image.Key, "_")

	image.CdnPath = fmt.Sprintf("/%s/%s/%s", conf.ResultStorage, processedKey, fileName)
	go action.SaveImageUrl(conf.MysqlServerConn, image, analytic)

	return finalPath
}
