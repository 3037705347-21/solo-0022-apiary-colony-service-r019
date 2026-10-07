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
// 该入口只处理低风险（即时生效）迁移；高风险目标必须走 RequestHiveState + DecideHiveState。
func (m *Memory) AdvanceHive(hiveID string, target domain.HiveState, _ time.Time) (domain.Transition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.advanceHiveLocked(hiveID, target)
}

// advanceHiveLocked 在持锁状态下执行蜂箱状态迁移校验与写入。
func (m *Memory) advanceHiveLocked(hiveID string, target domain.HiveState) (domain.Transition, error) {
	hive, ok := m.hives[hiveID]
	if !ok {
		return domain.Transition{}, domain.NotFound("蜂箱", hiveID)
	}
	if hive.State == target {
		return domain.Transition{}, domain.InvalidState("蜂箱 %q 已经处于状态 %q", hiveID, target)
	}
	if !domain.CanTransitionHive(hive.State, target) {
		return domain.Transition{}, domain.InvalidState(
			"蜂箱 %q 不允许从 %q 迁移到 %q", hiveID, hive.State, target)
	}
	from := hive.State
	hive.State = target
	m.hives[hiveID] = hive
	return domain.Transition{HiveID: hiveID, From: from, To: target}, nil
}

// RequestHiveState 为高风险目标建立待确认事项，事项写入时蜂箱状态保持不变。
// 合法性（目标状态、迁移表）在同一把写锁内按当前状态判定；同一蜂箱已有待确认事项时拒绝重复提交。
func (m *Memory) RequestHiveState(
	hiveID string,
	target domain.HiveState,
	requestedBy string,
	requestRole domain.ActorRole,
	now time.Time,
) (domain.StateConfirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return domain.StateConfirmation{}, domain.NotFound("蜂箱", hiveID)
	}
	if hive.State == target {
		return domain.StateConfirmation{}, domain.InvalidState("蜂箱 %q 已经处于状态 %q", hiveID, target)
	}
	if !domain.CanTransitionHive(hive.State, target) {
		return domain.StateConfirmation{}, domain.InvalidState(
			"蜂箱 %q 不允许从 %q 迁移到 %q", hiveID, hive.State, target)
	}
	for _, item := range m.confirmations[hiveID] {
		if item.IsPending() {
			return domain.StateConfirmation{}, domain.Conflict(
				"蜂箱 %q 已有待确认的状态推进事项 %q", hiveID, item.ID)
		}
	}
	m.confirmationSeq++
	confirmation := domain.StateConfirmation{
		ID:          fmt.Sprintf("confirmation-%06d", m.confirmationSeq),
		HiveID:      hiveID,
		From:        hive.State,
		To:          target,
		Status:      domain.ConfirmationPending,
		RequestedBy: requestedBy,
		RequestRole: requestRole,
		RequestedAt: now.UTC(),
	}
	m.confirmations[hiveID] = append(m.confirmations[hiveID], confirmation)
	return confirmation, nil
}

// DecideHiveState 对一个待确认事项给出唯一一次决定。
// 批准时在同一把写锁内复核蜂箱当前状态并完成迁移；拒绝或批准都只允许一次，
// 拒绝不产生任何蜂箱状态变化。
func (m *Memory) DecideHiveState(
	confirmationID string,
	approved bool,
	confirmedBy string,
	confirmerRole domain.ActorRole,
	reason string,
	now time.Time,
) (domain.StateConfirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hiveID, index, confirmation, err := m.findConfirmationLocked(confirmationID)
	if err != nil {
		return domain.StateConfirmation{}, err
	}
	if !confirmation.IsPending() {
		return domain.StateConfirmation{}, domain.Conflict(
			"确认事项 %q 已经被%s，不能重复确认", confirmationID, confirmationStatusLabel(confirmation.Status))
	}
	if confirmation.RequestedBy == confirmedBy || confirmation.RequestRole == confirmerRole {
		return domain.StateConfirmation{}, domain.Conflict(
			"确认事项 %q 必须由发起之外的另一个角色确认", confirmationID)
	}
	decidedAt := now.UTC()
	confirmation.ConfirmedBy = confirmedBy
	confirmation.ConfirmerRole = confirmerRole
	confirmation.ConfirmedAt = &decidedAt
	confirmation.Reason = reason
	if approved {
		transition, err := m.advanceHiveLocked(hiveID, confirmation.To)
		if err != nil {
			return domain.StateConfirmation{}, err
		}
		confirmation.Status = domain.ConfirmationApproved
		transitionCopy := transition
		confirmation.Transition = &transitionCopy
	} else {
		confirmation.Status = domain.ConfirmationRejected
	}
	m.confirmations[hiveID][index] = confirmation
	return confirmation, nil
}

func (m *Memory) findConfirmationLocked(confirmationID string) (string, int, domain.StateConfirmation, error) {
	for hiveID, items := range m.confirmations {
		for index, item := range items {
			if item.ID == confirmationID {
				return hiveID, index, item, nil
			}
		}
	}
	return "", 0, domain.StateConfirmation{}, domain.NotFound("状态推进确认事项", confirmationID)
}

func confirmationStatusLabel(status domain.ConfirmationStatus) string {
	switch status {
	case domain.ConfirmationApproved:
		return "批准"
	case domain.ConfirmationRejected:
		return "拒绝"
	default:
		return "处理"
	}
}

// ListConfirmations 返回一个蜂箱的状态推进确认历史，顺序按事项产生时间排列。
func (m *Memory) ListConfirmations(hiveID string) ([]domain.StateConfirmation, error) {
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
// 保证任意时刻同一蜂箱至多只有一只 laying 蜂王。当前活跃蜂王编号直接取
// 锁内的蜂箱快照，避免调用方在锁外先读后写与并发换王交错。
func (m *Memory) AddQueen(
	hiveID string,
	queen domain.Queen,
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
	if hive.ActiveQueenID != "" {
		if err := m.closeQueenLocked(hiveID, hive.ActiveQueenID, closeState, now); err != nil {
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
			if items[index].ConfirmedAt != nil {
				confirmedAt := *items[index].ConfirmedAt
				copied[index].ConfirmedAt = &confirmedAt
			}
			if items[index].Transition != nil {
				transition := *items[index].Transition
				copied[index].Transition = &transition
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
