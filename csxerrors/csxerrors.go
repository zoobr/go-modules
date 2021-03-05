package csxerrors

import (
	"strings"

	"github.com/sirupsen/logrus"
	"gitlab.com/battler/modules/csxhttp"
)

type ErrorItem struct {
	statusCode int
	messages   map[string]string
}

var (
	errorsItems = map[string]ErrorItem{}
)

type Lang struct {
	Key    string `db:"key" json:"key" key:"1"`
	Status int    `db:"status" json:"status" type:"int4"`
	Ru     string `db:"ru" json:"ru"`
	En     string `db:"en" json:"en"`
}

func getErrorMsg(errorCode string, lang string) (msg string, statusCode int) {
	if lang == "" {
		lang = "en"
	} else {
		langs := strings.SplitAfter(lang, ",")
		if len(langs) > 1 {
			lang = strings.Trim(langs[1], " ")
		}
	}

	item, ok := errorsItems[errorCode]
	if !ok {
		return errorCode, 400
	}
	itemMsg, ok := item.messages[lang]
	if !ok {
		return errorCode, 400
	}
	return itemMsg, item.statusCode
}

// GetErrorMsg is using for returning localized error messages & status
func GetErrorMsg(errorCode, locale string) (msg string, status int) {
	return getErrorMsg(errorCode, locale)
}

//Error is using for handling error responses
func Error(ctx *csxhttp.Context, errorCode string, err ...interface{}) error {
	lenErr := len(err)
	if lenErr > 0 {
		newErrs := make([]interface{}, 0)
		if lenErr > 1 {
			newErrs = append(newErrs, "[", err[0], "]", err[1:])
		} else {
			newErrs = append(newErrs, "[", err[0], "]")
		}
		logrus.Error(newErrs...)
	}
	lang := strings.Replace(ctx.GetHeader("Accept-Language"), " ", "", -1)
	parts := strings.Split(lang, ",")
	if len(parts) > 1 {
		lang = parts[1]
	}
	msg, status := getErrorMsg(errorCode, lang)
	return ctx.String(status, msg)
}

//ChatError is using for handling error chat responses
func ChatError(ctx *csxhttp.Context, errorCode string, clientID string, regStateID *string, err ...interface{}) error {
	if len(err) > 0 {
		logrus.Error(err...)
	}
	lang := strings.Replace(ctx.GetHeader("Accept-Language"), " ", "", -1)
	parts := strings.Split(lang, ",")
	if len(parts) > 1 {
		lang = parts[1]
	}
	msg, status := getErrorMsg(errorCode, lang)
	return ctx.String(status, msg)
}

//Success is using for handling success responses
func Success(ctx *csxhttp.Context, messageCode string, info ...interface{}) error {
	if len(info) > 0 {
		logrus.Info(info...)
	}
	lang := strings.Replace(ctx.GetHeader("Accept-Language"), " ", "", -1)
	parts := strings.Split(lang, ",")
	if len(parts) > 1 {
		lang = parts[1]
	}
	msg, status := getErrorMsg(messageCode, lang)
	return ctx.String(status, msg)
}

func Init(langs []Lang) {
	for _, item := range langs {
		errorsItems[item.Key] = ErrorItem{
			statusCode: item.Status,
			messages: map[string]string{
				"en": item.En,
				"ru": item.Ru,
			},
		}
	}
}
