package middleware

import (
	"net/http"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

func WithOutgoingMetadata(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-User-Id")

		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = uuid.New().String()
		}

		ctx := metadata.AppendToOutgoingContext(r.Context(), "x-user-id", userID, "x-request-id", reqID)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}
