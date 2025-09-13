package admin

import (
	"io"
	"log"
	"net/http"

	"github.com/glimesh/broadcast-box/internal/environment"
	"github.com/glimesh/broadcast-box/internal/server/helpers"
)

func AdminLoggingHandler(responseWriter http.ResponseWriter, request *http.Request) {
	if isValidMethod := verifyValidMethod("GET", responseWriter, request); !isValidMethod {
		return
	}

	sessionResult := verifyAdminSession(request)
	if !sessionResult.IsValid {
		helpers.LogHttpError(responseWriter, sessionResult.ErrorMessage, http.StatusUnauthorized)
		return
	}

	file, err := environment.GetLogFileReader()
	if err != nil {
		log.Println("API.Admin.Logging Error:", err)
	}

	responseWriter.Header().Set("Content-Type", "application/json")

	if _, err := io.Copy(responseWriter, file); err != nil {
		log.Println("API.Admin.Logging: Error writing file to response", err)
		helpers.LogHttpError(responseWriter, "Invalid request", http.StatusBadRequest)
	}

	err = file.Close()
	if err != nil {
		log.Println("API.Admin.Logging Error:", err)
	}
}
