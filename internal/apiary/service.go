package apiary

import (
	"time"

	"example.com/apiary-colony-service/internal/domain"
	"example.com/apiary-colony-service/internal/store"
)

// Service 负责蜂场与蜂箱的录入，以及蜂箱状态迁移。
type Service struct {
	store *store.Memory
	clock func() time.Time
}

func New(memory *store.Memory, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{store: memory, clock: clock}
}

type RegisterApiaryInput struct {
	ID       string
	Name     string
	District string
	Keeper   string
}

type InstallHiveInput struct {
	ID    string
	Model string
}

func (s *Service) RegisterApiary(input RegisterApiaryInput) (domain.Apiary, error) {
	apiary, err := domain.NewApiary(input.ID, input.Name, input.District, input.Keeper, s.clock())
	if err != nil {
		return domain.Apiary{}, err
	}
	if err := s.store.CreateApiary(apiary); err != nil {
		return domain.Apiary{}, err
	}
	return apiary, nil
}

func (s *Service) Apiary(id string) (domain.Apiary, error) {
	apiary, ok := s.store.GetApiary(id)
	if !ok {
		return domain.Apiary{}, domain.NotFound("蜂场", id)
	}
	return apiary, nil
}

func (s *Service) InstallHive(apiaryID string, input InstallHiveInput) (domain.Hive, error) {
	if _, err := s.Apiary(apiaryID); err != nil {
		return domain.Hive{}, err
	}
	model, err := ParseHiveModel(input.Model)
	if err != nil {
		return domain.Hive{}, err
	}
	hive, err := domain.NewHive(input.ID, apiaryID, model, s.clock())
	if err != nil {
		return domain.Hive{}, err
	}
	if err := s.store.CreateHive(hive); err != nil {
		return domain.Hive{}, err
	}
	return hive, nil
}

func (s *Service) Hive(id string) (domain.Hive, error) {
	hive, ok := s.store.GetHive(id)
	if !ok {
		return domain.Hive{}, domain.NotFound("蜂箱", id)
	}
	return hive, nil
}

type AdvanceHiveStateInput struct {
	Target      string
	RequestedBy string
	Role        string
}

// AdvanceHiveStateResult 区分即时生效的迁移与仅形成待确认事项两种结果。
type AdvanceHiveStateResult struct {
	Transition   *domain.Transition        `json:"transition,omitempty"`
	Confirmation *domain.StateConfirmation `json:"confirmation,omitempty"`
	// Pending 为 true 表示高风险目标仅形成待确认事项，蜂箱状态尚未改变。
	Pending bool `json:"pending"`
}

// ConfirmHiveStateInput 是另一个角色对待确认事项作出的一次决定。
type ConfirmHiveStateInput struct {
	ConfirmedBy string
	Role        string
	Decision    string
	Reason      string
}

func (s *Service) AdvanceHiveState(hiveID string, input AdvanceHiveStateInput) (AdvanceHiveStateResult, error) {
	to, err := ParseHiveState(input.Target)
	if err != nil {
		return AdvanceHiveStateResult{}, err
	}
	role, err := ParseActorRole("role", input.Role)
	if err != nil {
		return AdvanceHiveStateResult{}, err
	}
	if role != domain.RoleCoordinator {
		return AdvanceHiveStateResult{}, domain.Invalid(
			"role", "只有片区协调人（%s）可以发起蜂群状态推进", domain.RoleCoordinator)
	}
	requester, err := domain.ParseActorID(input.RequestedBy)
	if err != nil {
		return AdvanceHiveStateResult{}, err
	}
	now := s.clock()
	if domain.IsHighRiskTarget(to) {
		confirmation, err := s.store.RequestHiveState(hiveID, to, requester, role, now)
		if err != nil {
			return AdvanceHiveStateResult{}, err
		}
		return AdvanceHiveStateResult{Confirmation: &confirmation, Pending: true}, nil
	}
	transition, err := s.store.AdvanceHive(hiveID, to, now)
	if err != nil {
		return AdvanceHiveStateResult{}, err
	}
	return AdvanceHiveStateResult{Transition: &transition}, nil
}

// ConfirmHiveState 由发起之外的另一个角色对待确认事项作出唯一一次确认。
func (s *Service) ConfirmHiveState(confirmationID string, input ConfirmHiveStateInput) (domain.StateConfirmation, error) {
	role, err := ParseActorRole("role", input.Role)
	if err != nil {
		return domain.StateConfirmation{}, err
	}
	if role != domain.RoleKeeper {
		return domain.StateConfirmation{}, domain.Invalid(
			"role", "只有蜂场值守人（%s）可以确认高风险状态推进", domain.RoleKeeper)
	}
	confirmer, err := domain.ParseActorID(input.ConfirmedBy)
	if err != nil {
		return domain.StateConfirmation{}, err
	}
	approved, err := ParseConfirmationDecision(input.Decision)
	if err != nil {
		return domain.StateConfirmation{}, err
	}
	reason := domain.TrimText(input.Reason)
	if count := len([]rune(reason)); count > 200 {
		return domain.StateConfirmation{}, domain.Invalid("reason", "reason 长度不能超过 %d 个字符", 200)
	}
	return s.store.DecideHiveState(confirmationID, approved, confirmer, role, reason, s.clock())
}

// Confirmations 读取蜂箱的状态推进确认历史。
func (s *Service) Confirmations(hiveID string) ([]domain.StateConfirmation, error) {
	return s.store.ListConfirmations(hiveID)
}
