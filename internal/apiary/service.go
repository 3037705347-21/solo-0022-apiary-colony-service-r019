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
}

// AdvanceOutcome 是状态推进的结果：普通目标立即生效并返回迁移；
// 高风险目标只登记待确认事项，蜂箱状态在另一角色确认前保持不变。
type AdvanceOutcome struct {
	Transition   *domain.Transition
	Confirmation *domain.StateConfirmation
}

func (s *Service) AdvanceHiveState(hiveID string, input AdvanceHiveStateInput) (AdvanceOutcome, error) {
	target, err := ParseHiveState(input.Target)
	if err != nil {
		return AdvanceOutcome{}, err
	}
	if !domain.RequiresConfirmation(target) {
		transition, err := s.store.AdvanceHive(hiveID, target, s.clock())
		if err != nil {
			return AdvanceOutcome{}, err
		}
		return AdvanceOutcome{Transition: &transition}, nil
	}
	requestedBy, err := ParseRole("requestedBy", input.RequestedBy)
	if err != nil {
		return AdvanceOutcome{}, err
	}
	confirmation, err := s.store.RequestHiveAdvance(hiveID, target, requestedBy, s.clock())
	if err != nil {
		return AdvanceOutcome{}, err
	}
	return AdvanceOutcome{Confirmation: &confirmation}, nil
}

type DecideStateConfirmationInput struct {
	Decision  string
	DecidedBy string
}

// DecideOutcome 是确认处理的结果：确认时附带实际写入的迁移，拒绝时只有确认记录。
type DecideOutcome struct {
	Confirmation domain.StateConfirmation
	Transition   *domain.Transition
}

func (s *Service) DecideStateConfirmation(
	hiveID string,
	confirmationID string,
	input DecideStateConfirmationInput,
) (DecideOutcome, error) {
	decision, err := ParseDecision(input.Decision)
	if err != nil {
		return DecideOutcome{}, err
	}
	decidedBy, err := ParseRole("decidedBy", input.DecidedBy)
	if err != nil {
		return DecideOutcome{}, err
	}
	confirmation, transition, err := s.store.DecideHiveAdvance(
		hiveID, confirmationID, decision, decidedBy, s.clock())
	if err != nil {
		return DecideOutcome{}, err
	}
	return DecideOutcome{Confirmation: confirmation, Transition: transition}, nil
}

func (s *Service) ListStateConfirmations(hiveID string) ([]domain.StateConfirmation, error) {
	return s.store.ListStateConfirmations(hiveID)
}
