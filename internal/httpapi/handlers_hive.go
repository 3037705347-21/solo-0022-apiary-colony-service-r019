package httpapi

import (
	"net/http"

	"example.com/apiary-colony-service/internal/apiary"
	"example.com/apiary-colony-service/internal/domain"
)

type installHiveRequest struct {
	ID    string `json:"id"`
	Model string `json:"model"`
}

type advanceHiveStateRequest struct {
	Target      string `json:"target"`
	RequestedBy string `json:"requestedBy"`
}

type decideStateConfirmationRequest struct {
	Decision  string `json:"decision"`
	DecidedBy string `json:"decidedBy"`
}

func (s *Server) handleInstallHive(w http.ResponseWriter, r *http.Request) {
	var request installHiveRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err)
		return
	}
	created, err := s.deps.Apiary.InstallHive(r.PathValue("apiaryId"), apiary.InstallHiveInput{
		ID:    request.ID,
		Model: request.Model,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleGetHive(w http.ResponseWriter, r *http.Request) {
	found, err := s.deps.Apiary.Hive(r.PathValue("hiveId"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, found)
}

func (s *Server) handleAdvanceHiveState(w http.ResponseWriter, r *http.Request) {
	var request advanceHiveStateRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err)
		return
	}
	outcome, err := s.deps.Apiary.AdvanceHiveState(r.PathValue("hiveId"), apiary.AdvanceHiveStateInput{
		Target:      request.Target,
		RequestedBy: request.RequestedBy,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	if outcome.Confirmation != nil {
		writeJSON(w, http.StatusAccepted, outcome.Confirmation)
		return
	}
	writeJSON(w, http.StatusOK, outcome.Transition)
}

func (s *Server) handleDecideStateConfirmation(w http.ResponseWriter, r *http.Request) {
	var request decideStateConfirmationRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err)
		return
	}
	outcome, err := s.deps.Apiary.DecideStateConfirmation(
		r.PathValue("hiveId"),
		r.PathValue("confirmationId"),
		apiary.DecideStateConfirmationInput{
			Decision:  request.Decision,
			DecidedBy: request.DecidedBy,
		},
	)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, outcome.Confirmation)
}

func (s *Server) handleListStateConfirmations(w http.ResponseWriter, r *http.Request) {
	items, err := s.deps.Apiary.ListStateConfirmations(r.PathValue("hiveId"))
	if err != nil {
		writeError(w, err)
		return
	}
	if items == nil {
		items = []domain.StateConfirmation{}
	}
	writeJSON(w, http.StatusOK, items)
}
