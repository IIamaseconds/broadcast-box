package admin

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/glimesh/broadcast-box/internal/server/helpers"
	"github.com/glimesh/broadcast-box/internal/webrtc/session"
)

func AdminStatusHandler(responseWriter http.ResponseWriter, request *http.Request) {
	if isValidMethod := verifyValidMethod("GET", responseWriter, request); !isValidMethod {
		return
	}

	sessionResult := verifyAdminSession(request)
	if !sessionResult.IsValid {
		helpers.LogHttpError(responseWriter, sessionResult.ErrorMessage, http.StatusUnauthorized)
		return
	}

	sessions := session.SessionManager.GetSessionStates(true)

	responseWriter.Header().Set("Content-Type", "application/json")

	err := json.NewEncoder(responseWriter).Encode(sessions)
	if err != nil {
		log.Println("API.AdminStatus Error", err)
	}
}
