package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/glimesh/broadcast-box/internal/environment"
	"github.com/glimesh/broadcast-box/internal/server/helpers"
	"github.com/glimesh/broadcast-box/internal/webrtc/sessions/manager"
	"github.com/google/uuid"
)

func sseHandler(responseWriter http.ResponseWriter, request *http.Request) {
	flusher, ok := responseWriter.(http.Flusher)
	if !ok {
		http.Error(responseWriter, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	responseWriter.Header().Add("Content-Type", "text/event-stream")
	responseWriter.Header().Add("Cache-Control", "no-cache")
	responseWriter.Header().Add("Connection", "keep-alive")

	values := strings.Split(request.URL.RequestURI(), "/")
	sessionID := values[len(values)-1]

	debugSseMessages := strings.EqualFold(os.Getenv(environment.DebugPrintSSEMessages), "true")
	writeTimeout := 500 * time.Millisecond

	ctx := request.Context()
	responseController := http.NewResponseController(responseWriter)

	var writeLock sync.Mutex
	writeEvent := func(writeCtx context.Context, msg string) bool {
		if msg == "" || writeCtx.Err() != nil {
			return false
		}

		writeLock.Lock()
		defer writeLock.Unlock()

		if debugSseMessages {
			log.Println("API.SSE Sending:", msg)
		}

		if err := responseController.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			log.Println("API.SSE SetWriteDeadline error:", err)
			return false
		}

		_, err := fmt.Fprintf(responseWriter, "%s\n", msg)
		if err == nil {
			flusher.Flush()
		}

		if deadlineErr := responseController.SetWriteDeadline(time.Time{}); deadlineErr != nil && !errors.Is(deadlineErr, http.ErrNotSupported) {
			log.Println("API.SSE ClearWriteDeadline error:", deadlineErr)
			return false
		}

		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				log.Println("API.SSE Write timeout")
			} else {
				log.Println("API.SSE Write error:", err)
			}
			return false
		}

		return true
	}

	if streamSession, whepSession, foundSession := manager.SessionsManager.GetSessionAndWHEPByID(sessionID); foundSession {
		subscriberCtx, subscriberCancel := context.WithCancel(ctx)
		defer subscriberCancel()

		subscriberID := uuid.NewString()
		subscriberWrite := func(msg string) bool {
			return writeEvent(subscriberCtx, msg)
		}
		if !whepSession.AddSSESubscriber(subscriberID, subscriberWrite, subscriberCancel) {
			helpers.LogHTTPError(responseWriter, "Invalid request", http.StatusBadRequest)
			return
		}
		defer whepSession.RemoveSSESubscriber(subscriberID)

		if !subscriberWrite(streamSession.GetSessionStatsEvent()) {
			return
		}

		host := streamSession.Host.Load()
		if host != nil && !subscriberWrite(host.GetAvailableLayersEvent()) {
			return
		}

		<-subscriberCtx.Done()
		log.Println("API.SSE: Client disconnected")
		return
	}

	if streamSession, foundSession := manager.SessionsManager.GetSessionByHostSessionID(sessionID); foundSession {
		if !writeEvent(ctx, streamSession.GetSessionStatsEvent()) {
			return
		}

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("API.SSE: Client disconnected")
				return
			case <-ticker.C:
				if !writeEvent(ctx, streamSession.GetSessionStatsEvent()) {
					return
				}
			}
		}
	}

	helpers.LogHTTPError(responseWriter, "Invalid request", http.StatusBadRequest)
}
