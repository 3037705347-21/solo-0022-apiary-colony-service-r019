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

// ParseActorRole 把入口层字符串收敛为受支持的参与角色。
func ParseActorRole(field, value string) (domain.ActorRole, error) {
	role := domain.ActorRole(domain.TrimText(value))
	if !domain.ValidActorRole(role) {
		return "", domain.Invalid(field, "%s 必须是 %s 或 %s", field, domain.RoleCoordinator, domain.RoleKeeper)
	}
	return role, nil
}

// ParseConfirmationDecision 把确认入口的决定字符串收敛为布尔语义。
func ParseConfirmationDecision(value string) (bool, error) {
	switch domain.TrimText(value) {
	case "approve":
		return true, nil
	case "reject":
		return false, nil
	default:
		return false, domain.Invalid("decision", "decision 必须是 approve 或 reject")
	}
}
