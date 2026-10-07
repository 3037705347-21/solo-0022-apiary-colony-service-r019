package domain

import "time"

// ActorRole 是参与蜂群状态推进的角色。
type ActorRole string

const (
	// RoleCoordinator 片区协调人：发起蜂群状态推进。
	RoleCoordinator ActorRole = "coordinator"
	// RoleKeeper 蜂场值守人：确认高风险状态推进。
	RoleKeeper ActorRole = "keeper"
)

func ValidActorRole(role ActorRole) bool {
	switch role {
	case RoleCoordinator, RoleKeeper:
		return true
	default:
		return false
	}
}

// ConfirmationStatus 是高风险推进待确认事项的处理状态。
type ConfirmationStatus string

const (
	ConfirmationPending  ConfirmationStatus = "pending"
	ConfirmationApproved ConfirmationStatus = "approved"
	ConfirmationRejected ConfirmationStatus = "rejected"
)

// StateConfirmation 是一次高风险状态推进在正式生效前形成的待确认事项。
// 事项存在期间不改变蜂箱状态；只有被另一个角色批准后才附带一次实际迁移。
type StateConfirmation struct {
	ID            string             `json:"id"`
	HiveID        string             `json:"hiveId"`
	From          HiveState          `json:"from"`
	To            HiveState          `json:"to"`
	Status        ConfirmationStatus `json:"status"`
	RequestedBy   string             `json:"requestedBy"`
	RequestRole   ActorRole          `json:"requestRole"`
	RequestedAt   time.Time          `json:"requestedAt"`
	ConfirmedBy   string             `json:"confirmedBy,omitempty"`
	ConfirmerRole ActorRole          `json:"confirmerRole,omitempty"`
	ConfirmedAt   *time.Time         `json:"confirmedAt,omitempty"`
	Reason        string             `json:"reason,omitempty"`
	// Transition 仅在批准后写入，记录实际发生的迁移。
	Transition *Transition `json:"transition,omitempty"`
}

func (c StateConfirmation) IsPending() bool {
	return c.Status == ConfirmationPending
}

// IsHighRiskTarget 判断目标状态是否为高风险推进。
// retired 是不可回退的终态，一次误操作无法撤销，是当前唯一需要二次确认的目标。
func IsHighRiskTarget(to HiveState) bool {
	return to == HiveRetired
}

// ParseActorID 收敛并校验状态推进参与人的身份标识。
func ParseActorID(value string) (string, error) {
	if err := ValidateIdentifier("actorId", value, 2, 40); err != nil {
		return "", err
	}
	return TrimText(value), nil
}
