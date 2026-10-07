package apiary

import "example.com/apiary-colony-service/internal/domain"

// ParseHiveModel 把入口层字符串收敛为受支持的箱型。
func ParseHiveModel(value string) (domain.HiveModel, error) {
	model := domain.HiveModel(domain.TrimText(value))
	if !domain.ValidHiveModel(model) {
		return "", domain.Invalid(
			"model",
			"model 必须是 %s、%s 或 %s",
			domain.ModelLangstroth, domain.ModelTopBar, domain.ModelWarre,
		)
	}
	return model, nil
}

// ParseHiveState 把入口层字符串收敛为受支持的蜂群状态。
func ParseHiveState(value string) (domain.HiveState, error) {
	state := domain.HiveState(domain.TrimText(value))
	if !domain.ValidHiveState(state) {
		return "", domain.Invalid(
			"target",
			"target 必须是 %s、%s、%s 或 %s",
			domain.HiveColonized, domain.HiveProduction, domain.HiveWintering, domain.HiveRetired,
		)
	}
	return state, nil
}

// ParseRole 把入口层字符串收敛为受支持的角色。
func ParseRole(field, value string) (domain.Role, error) {
	role := domain.Role(domain.TrimText(value))
	if !domain.ValidRole(role) {
		return "", domain.Invalid(
			field,
			"%s 必须是 %s 或 %s",
			field, domain.RoleKeeper, domain.RoleCoordinator,
		)
	}
	return role, nil
}

// ParseDecision 把入口层字符串收敛为受支持的确认决定。
func ParseDecision(value string) (domain.Decision, error) {
	decision := domain.Decision(domain.TrimText(value))
	if !domain.ValidDecision(decision) {
		return "", domain.Invalid(
			"decision",
			"decision 必须是 %s 或 %s",
			domain.DecisionConfirm, domain.DecisionReject,
		)
	}
	return decision, nil
}
