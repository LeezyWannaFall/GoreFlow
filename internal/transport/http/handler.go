package httptransport

import (
	"errors"
	"net/http"

	"github.com/LeezyWannaFall/GoreFlow/internal/application"
	"github.com/LeezyWannaFall/GoreFlow/internal/job"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	app JobService
}

func NewHandler(app JobService) *Handler {
	return &Handler{app: app}
}

func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var request CreateJobRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	model, err := h.app.CreateJob(r.Context(), request.Type, request.Payload)
	if err != nil {
		if errors.Is(err, job.ErrInvalidJobType) || errors.Is(err, job.ErrInvalidPayload) {
			writeError(w, http.StatusBadRequest, "invalid job data")
			return
		}

		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusCreated, newJobResponse(model))
}

func (h *Handler) GetJobByID(w http.ResponseWriter, r *http.Request) {
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || jobID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}

	model, err := h.app.GetJobByID(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, application.ErrJobNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}

		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, newJobResponse(model))
}
