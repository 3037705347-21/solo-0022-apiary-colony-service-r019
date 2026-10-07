package domain

import "time"

// Role 标识参与状态推进的角色，高风险迁移要求确认人与提交人角色不同。
type Role string

const (
	RoleKeeper      Role = "keeper"
	RoleCoordinator Role = "coordinator"
)

// Decision 是对待确认事项的处理决定。
type Decision string

const (
	DecisionConfirm Decision = "confirm"
	DecisionReject  Decision = "reject"
)

type ConfirmationState string

const (
	ConfirmationPending   ConfirmationState = "pending"
	ConfirmationConfirmed ConfirmationState = "confirmed"
	ConfirmationRejected  ConfirmationState = "rejected"
)

// StateConfirmation 是一次高风险状态迁移的待确认事项。
// 确认前蜂箱状态不变；确认后迁移生效且不可再次处理；被拒绝时不留下状态变化。
type StateConfirmation struct {
	ID          string            `json:"id"`
	HiveID      string            `json:"hiveId"`
	From        HiveState         `json:"from"`
	To          HiveState         `json:"to"`
	RequestedBy Role              `json:"requestedBy"`
	RequestedAt time.Time         `json:"requestedAt"`
	State       ConfirmationState `json:"state"`
	DecidedBy   Role              `json:"decidedBy,omitempty"`
	DecidedAt   *time.Time        `json:"decidedAt,omitempty"`
}

func ValidRole(role Role) bool {
	switch role {
	case RoleKeeper, RoleCoordinator:
		return true
	default:
		return false
	}
}

func ValidDecision(decision Decision) bool {
	switch decision {
	case DecisionConfirm, DecisionReject:
		return true
	default:
		return false
	}
}

func (c StateConfirmation) IsPending() bool {
	return c.State == ConfirmationPending
}
