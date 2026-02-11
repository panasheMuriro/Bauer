package v1

import (
	"bauer/cmd/app/models/v1"
	"bauer/cmd/app/types"
	"bauer/internal/config"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

func JobPost(rc types.RouteConfig) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Context().Value("requestID").(string)
		payload := models.JobPost{}
		err := json.NewDecoder(r.Body).Decode(&payload)
		if err != nil {
			slog.Error("failed to decode request body", "error", err.Error(), "requestID", requestID)
			err := types.BadRequest(fmt.Errorf("invalid request body: %w", err)).Render(w, r)
			if err != nil {
				slog.Error("error writing response", "error", err.Error(), "requestID", requestID)
			}
			return
		}
		cfg := config.Config {
			DocID: payload.DocID,
			ChunkSize: payload.ChunkSize,
			PageRefresh: payload.PageRefresh,
			CredentialsPath: rc.APIConfig.CredentialsPath,
			OutputDir: fmt.Sprintf("%s/%s", rc.APIConfig.BaseOutputDir, requestID),
			Model: rc.APIConfig.Model,
			SummaryModel: rc.APIConfig.SummaryModel,
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, "requestID", &requestID)

		_, err = rc.Orchestrator.Execute(ctx, &cfg)
		if err != nil {
			err := types.InternalError(fmt.Errorf("failed to execute job")).Render(w, r)
			if err != nil {
				slog.Error("error writing response", "error", err.Error(), "requestID", requestID)
			}
			return
		}
		slog.Info("job executed successfully", "requestID", requestID)
		err = types.Success().Render(w, r)
		if err != nil {
			slog.Error("error writing response", "error", err.Error(), "requestID", requestID)
		}
	}
}
