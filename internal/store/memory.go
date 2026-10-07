package store

import (
	"fmt"
	"sync"
	"time"

	"example.com/apiary-colony-service/internal/domain"
)

// Memory 是并发安全的内存仓储。所有写入都在写锁内完成状态校验，
// 读取则通过 Snapshot 得到一致视图。
type Memory struct {
	mu sync.RWMutex

	apiaries      map[string]domain.Apiary
	hives         map[string]domain.Hive
	inspections   map[string][]domain.Inspection
	queens        map[string][]domain.Queen
	confirmations map[string][]domain.StateConfirmation

	inspectionSeq   int
	queenSeq        int
	confirmationSeq int
}

func NewMemory() *Memory {
	return &Memory{
		apiaries:      make(map[string]domain.Apiary),
		hives:         make(map[string]domain.Hive),
		inspections:   make(map[string][]domain.Inspection),
		queens:        make(map[string][]domain.Queen),
		confirmations: make(map[string][]domain.StateConfirmation),
	}
}

func (m *Memory) CreateApiary(apiary domain.Apiary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.apiaries[apiary.ID]; ok {
		return domain.Conflict("蜂场 %q 已存在", apiary.ID)
	}
	m.apiaries[apiary.ID] = apiary
	return nil
}

func (m *Memory) GetApiary(id string) (domain.Apiary, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	apiary, ok := m.apiaries[id]
	return apiary, ok
}

func (m *Memory) CreateHive(hive domain.Hive) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.apiaries[hive.ApiaryID]; !ok {
		return domain.NotFound("蜂场", hive.ApiaryID)
	}
	if _, ok := m.hives[hive.ID]; ok {
		return domain.Conflict("蜂箱 %q 已存在", hive.ID)
	}
	m.hives[hive.ID] = hive
	return nil
}

func (m *Memory) GetHive(id string) (domain.Hive, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	hive, ok := m.hives[id]
	return hive, ok
}

// AdvanceHive 在同一把写锁内完成状态读取、迁移校验与写入，避免并发迁移产生交错结果。
func (m *Memory) AdvanceHive(hiveID string, target domain.HiveState, _ time.Time) (domain.Transition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return domain.Transition{}, domain.NotFound("蜂箱", hiveID)
	}
	if err := checkHiveTransition(hive, target); err != nil {
		return domain.Transition{}, err
	}
	from := hive.State
	hive.State = target
	m.hives[hiveID] = hive
	return domain.Transition{HiveID: hiveID, From: from, To: target}, nil
}

func checkHiveTransition(hive domain.Hive, target domain.HiveState) error {
	if hive.State == target {
		return domain.InvalidState("蜂箱 %q 已经处于状态 %q", hive.ID, target)
	}
	if !domain.CanTransitionHive(hive.State, target) {
		return domain.InvalidState(
			"蜂箱 %q 不允许从 %q 迁移到 %q", hive.ID, hive.State, target)
	}
	return nil
}

// RequestHiveAdvance 校验高风险迁移并登记待确认事项，蜂箱状态保持不变。
// 同一蜂箱同时只允许一个待确认事项，判定与写入处于同一把锁。
func (m *Memory) RequestHiveAdvance(
	hiveID string,
	target domain.HiveState,
	requestedBy domain.Role,
	now time.Time,
) (domain.StateConfirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return domain.StateConfirmation{}, domain.NotFound("蜂箱", hiveID)
	}
	if err := checkHiveTransition(hive, target); err != nil {
		return domain.StateConfirmation{}, err
	}
	for _, item := range m.confirmations[hiveID] {
		if item.IsPending() {
			return domain.StateConfirmation{}, domain.Conflict(
				"蜂箱 %q 已存在待确认的状态迁移 %q", hiveID, item.ID)
		}
	}
	m.confirmationSeq++
	confirmation := domain.StateConfirmation{
		ID:          fmt.Sprintf("confirmation-%06d", m.confirmationSeq),
		HiveID:      hiveID,
		From:        hive.State,
		To:          target,
		RequestedBy: requestedBy,
		RequestedAt: now.UTC(),
		State:       domain.ConfirmationPending,
	}
	m.confirmations[hiveID] = append(m.confirmations[hiveID], confirmation)
	return confirmation, nil
}

// DecideHiveAdvance 处理待确认事项。确认时在同一把写锁内重新校验迁移并写入新状态，
// 保证确认记录与蜂箱状态同时变化；拒绝时只登记决定，不留下任何状态变化。
// 已处理的确认不能再次处理，确认人必须与提交人角色不同。
func (m *Memory) DecideHiveAdvance(
	hiveID string,
	confirmationID string,
	decision domain.Decision,
	decidedBy domain.Role,
	now time.Time,
) (domain.StateConfirmation, *domain.Transition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return domain.StateConfirmation{}, nil, domain.NotFound("蜂箱", hiveID)
	}
	items := m.confirmations[hiveID]
	index := -1
	for i, item := range items {
		if item.ID == confirmationID {
			index = i
			break
		}
	}
	if index < 0 {
		return domain.StateConfirmation{}, nil, domain.NotFound("状态确认", confirmationID)
	}
	confirmation := items[index]
	if !confirmation.IsPending() {
		return domain.StateConfirmation{}, nil, domain.Conflict(
			"状态确认 %q 已处理，不能重复处理", confirmationID)
	}
	if decidedBy == confirmation.RequestedBy {
		return domain.StateConfirmation{}, nil, domain.Conflict(
			"状态确认 %q 需要由与提交人不同的角色确认", confirmationID)
	}
	decidedAt := now.UTC()
	confirmation.DecidedBy = decidedBy
	confirmation.DecidedAt = &decidedAt
	if decision == domain.DecisionReject {
		confirmation.State = domain.ConfirmationRejected
		items[index] = confirmation
		m.confirmations[hiveID] = items
		return confirmation, nil, nil
	}
	if err := checkHiveTransition(hive, confirmation.To); err != nil {
		return domain.StateConfirmation{}, nil, err
	}
	from := hive.State
	hive.State = confirmation.To
	m.hives[hiveID] = hive
	confirmation.State = domain.ConfirmationConfirmed
	items[index] = confirmation
	m.confirmations[hiveID] = items
	transition := domain.Transition{HiveID: hiveID, From: from, To: confirmation.To}
	return confirmation, &transition, nil
}

// ListStateConfirmations 返回蜂箱的状态确认历史，按登记顺序排列。
func (m *Memory) ListStateConfirmations(hiveID string) ([]domain.StateConfirmation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.hives[hiveID]; !ok {
		return nil, domain.NotFound("蜂箱", hiveID)
	}
	items := m.confirmations[hiveID]
	out := make([]domain.StateConfirmation, len(items))
	copy(out, items)
	return out, nil
}

// AddInspection 分配巡检编号、写入记录，并推进蜂箱的最近巡检时间。
// 蜂箱存在与可接受巡检的判定与写入处于同一把锁，避免退役与巡检之间的竞态。
func (m *Memory) AddInspection(hiveID string, inspection domain.Inspection) (domain.Inspection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return domain.Inspection{}, domain.NotFound("蜂箱", hiveID)
	}
	if !hive.AcceptsInspection() {
		return domain.Inspection{}, domain.InvalidState("蜂箱 %q 处于 %q 状态，不接受巡检", hiveID, hive.State)
	}
	m.inspectionSeq++
	inspection.ID = fmt.Sprintf("inspection-%06d", m.inspectionSeq)
	inspection.HiveID = hiveID
	m.inspections[hiveID] = append(m.inspections[hiveID], inspection)

	observedAt := inspection.ObservedAt
	hive.LastInspectionAt = &observedAt
	m.hives[hiveID] = hive
	return inspection, nil
}

// AddQueen 关闭原有产卵蜂王并写入新蜂王，两件事在同一把锁内完成，
// 保证任意时刻同一蜂箱至多只有一只 laying 蜂王。
func (m *Memory) AddQueen(
	hiveID string,
	queen domain.Queen,
	closeQueenID string,
	closeState domain.QueenState,
	now time.Time,
) (domain.Queen, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return domain.Queen{}, domain.NotFound("蜂箱", hiveID)
	}
	if !hive.AcceptsQueenChange() {
		return domain.Queen{}, domain.InvalidState("蜂箱 %q 处于 %q 状态，不接受换王", hiveID, hive.State)
	}
	if closeQueenID != "" {
		if err := m.closeQueenLocked(hiveID, closeQueenID, closeState, now); err != nil {
			return domain.Queen{}, err
		}
	}
	m.queenSeq++
	queen.ID = fmt.Sprintf("queen-%06d", m.queenSeq)
	queen.HiveID = hiveID
	queen.State = domain.QueenLaying
	m.queens[hiveID] = append(m.queens[hiveID], queen)

	hive.ActiveQueenID = queen.ID
	m.hives[hiveID] = hive
	return queen, nil
}

func (m *Memory) closeQueenLocked(hiveID, queenID string, state domain.QueenState, now time.Time) error {
	items := m.queens[hiveID]
	for index, item := range items {
		if item.ID != queenID {
			continue
		}
		if !item.IsLaying() {
			return domain.Conflict("蜂王 %q 已经关闭", queenID)
		}
		closedAt := now.UTC()
		item.State = state
		item.ClosedAt = &closedAt
		items[index] = item
		m.queens[hiveID] = items
		return nil
	}
	return domain.Storage("蜂箱 %q 上的蜂王 %q 记录缺失", hiveID, queenID)
}

func (m *Memory) ListInspections(hiveID string) ([]domain.Inspection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.hives[hiveID]; !ok {
		return nil, domain.NotFound("蜂箱", hiveID)
	}
	items := m.inspections[hiveID]
	out := make([]domain.Inspection, len(items))
	copy(out, items)
	return out, nil
}

func (m *Memory) ListQueens(hiveID string) ([]domain.Queen, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.hives[hiveID]; !ok {
		return nil, domain.NotFound("蜂箱", hiveID)
	}
	items := m.queens[hiveID]
	out := make([]domain.Queen, len(items))
	copy(out, items)
	return out, nil
}

// Snapshot 返回深拷贝的一致视图，供汇总类读取使用。
func (m *Memory) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot := Snapshot{
		Apiaries:            make(map[string]domain.Apiary, len(m.apiaries)),
		Hives:               make(map[string]domain.Hive, len(m.hives)),
		InspectionsByHive:   make(map[string][]domain.Inspection, len(m.inspections)),
		QueensByHive:        make(map[string][]domain.Queen, len(m.queens)),
		ConfirmationsByHive: make(map[string][]domain.StateConfirmation, len(m.confirmations)),
	}
	for id, apiary := range m.apiaries {
		snapshot.Apiaries[id] = apiary
	}
	for id, hive := range m.hives {
		snapshot.Hives[id] = cloneHive(hive)
	}
	for id, items := range m.inspections {
		copied := make([]domain.Inspection, len(items))
		copy(copied, items)
		for index := range copied {
			copied[index].Concerns = append([]string{}, items[index].Concerns...)
		}
		snapshot.InspectionsByHive[id] = copied
	}
	for id, items := range m.queens {
		copied := make([]domain.Queen, len(items))
		copy(copied, items)
		for index := range copied {
			if items[index].ClosedAt != nil {
				closedAt := *items[index].ClosedAt
				copied[index].ClosedAt = &closedAt
			}
		}
		snapshot.QueensByHive[id] = copied
	}
	for id, items := range m.confirmations {
		copied := make([]domain.StateConfirmation, len(items))
		copy(copied, items)
		for index := range copied {
			if items[index].DecidedAt != nil {
				decidedAt := *items[index].DecidedAt
				copied[index].DecidedAt = &decidedAt
			}
		}
		snapshot.ConfirmationsByHive[id] = copied
	}
	return snapshot
}

func cloneHive(hive domain.Hive) domain.Hive {
	if hive.LastInspectionAt != nil {
		observedAt := *hive.LastInspectionAt
		hive.LastInspectionAt = &observedAt
	}
	return hive
}
