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
	Role        string `json:"role"`
}

type confirmationDecisionRequest struct {
	ConfirmedBy string `json:"confirmedBy"`
	Role        string `json:"role"`
	Decision    string `json:"decision"`
	Reason      string `json:"reason"`
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
	result, err := s.deps.Apiary.AdvanceHiveState(r.PathValue("hiveId"), apiary.AdvanceHiveStateInput{
		Target:      request.Target,
		RequestedBy: request.RequestedBy,
		Role:        request.Role,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	// 高风险目标只形成待确认事项，蜂箱状态尚未改变，用 202 与即时生效的 200 区分。
	if result.Pending {
		writeJSON(w, http.StatusAccepted, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleListConfirmations(w http.ResponseWriter, r *http.Request) {
	items, err := s.deps.Apiary.Confirmations(r.PathValue("hiveId"))
	if err != nil {
		writeError(w, err)
		return
	}
	if items == nil {
		items = []domain.StateConfirmation{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleConfirmationDecision(w http.ResponseWriter, r *http.Request) {
	var request confirmationDecisionRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err)
		return
	}
	confirmation, err := s.deps.Apiary.ConfirmHiveState(r.PathValue("confirmationId"), apiary.ConfirmHiveStateInput{
		ConfirmedBy: request.ConfirmedBy,
		Role:        request.Role,
		Decision:    request.Decision,
		Reason:      request.Reason,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, confirmation)
}
