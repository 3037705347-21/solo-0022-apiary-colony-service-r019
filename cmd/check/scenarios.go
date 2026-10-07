package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	demoApiaryID = "apiary-north"
	demoHiveID   = "hive-01"
)

// retireRequest 是协调人发起退役推进的标准请求体。
func retireRequest(by string) map[string]any {
	return map[string]any{"target": "retired", "requestedBy": by, "role": "coordinator"}
}

// decisionRequest 是值守人对待确认事项作出决定的标准请求体。
func decisionRequest(by, decision string) map[string]any {
	return map[string]any{"confirmedBy": by, "role": "keeper", "decision": decision}
}

// confirmationID 从发起退役的响应中提取待确认事项编号。
func confirmationID(reply *response) (string, error) {
	var body struct {
		Confirmation struct {
			ID string `json:"id"`
		} `json:"confirmation"`
	}
	if err := json.Unmarshal(reply.body, &body); err != nil {
		return "", fmt.Errorf("解析待确认事项响应失败：%v：%s", err, reply.text())
	}
	if body.Confirmation.ID == "" {
		return "", fmt.Errorf("响应缺少待确认事项编号：%s", reply.text())
	}
	return body.Confirmation.ID, nil
}

// decidePath 构造待确认事项决定入口。
func decidePath(id string) string {
	return "/state-confirmations/" + id + "/decision"
}

func seedApiary(c *client, id, name, district, keeper string) error {
	_, err := c.expect("POST", "/apiaries", map[string]any{
		"id":       id,
		"name":     name,
		"district": district,
		"keeper":   keeper,
	}, 201, `"id":"`+id+`"`)
	return err
}

func seedHive(c *client, apiaryID, hiveID, model string) error {
	_, err := c.expect("POST", "/apiaries/"+apiaryID+"/hives", map[string]any{
		"id":    hiveID,
		"model": model,
	}, 201, `"state":"colonized"`)
	return err
}

func seedDefault(c *client) error {
	if err := seedApiary(c, demoApiaryID, "北岭蜂场", "北岭片区", "周岚"); err != nil {
		return err
	}
	return seedHive(c, demoApiaryID, demoHiveID, "langstroth")
}

func checkRegisterApiary(c *client) error {
	if _, err := c.expect("POST", "/apiaries", map[string]any{
		"id": "apiary-north", "name": "北岭蜂场", "district": "北岭片区", "keeper": "周岚",
	}, 201, `"id":"apiary-north"`, `"name":"北岭蜂场"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/apiaries/apiary-north", nil, 200,
		`"district":"北岭片区"`, `"keeper":"周岚"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries", map[string]any{
		"id": "apiary-north", "name": "重复蜂场", "district": "北岭片区", "keeper": "周岚",
	}, 409, `"code":"conflict"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries", map[string]any{
		"id": "apiary-south", "name": "", "district": "南坡片区", "keeper": "李澈",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	return nil
}

func checkHiveInstall(c *client) error {
	if err := seedApiary(c, demoApiaryID, "北岭蜂场", "北岭片区", "周岚"); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-north/hives", map[string]any{
		"id": "hive-01", "model": "langstroth",
	}, 201, `"state":"colonized"`, `"apiaryId":"apiary-north"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"model":"langstroth"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-north/hives", map[string]any{
		"id": "hive-01", "model": "warre",
	}, 409, `"code":"conflict"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-missing/hives", map[string]any{
		"id": "hive-09", "model": "warre",
	}, 404, `"code":"not_found"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-north/hives", map[string]any{
		"id": "hive-02", "model": "dadant",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	return nil
}

func checkRecordInspection(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 9, "broodPattern": "patchy",
		"temper": "calm", "miteCount": 3, "note": "",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 3, "broodPattern": "patchy",
		"temper": "aggressive", "miteCount": 12, "note": "西侧巢门观察",
	}, 201, `"inspection-000001"`, `"mite_load"`, `"temper_risk"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01/inspections", nil, 200, `"note":"西侧巢门观察"`); err != nil {
		return err
	}
	// 退役是高风险目标：只形成待确认事项，蜂箱仍处于 colonized，巡检依然可以提交。
	pending, err := c.expect("POST", "/hives/hive-01/state", retireRequest("coord-li"), 202,
		`"pending":true`, `"status":"pending"`, `"to":"retired"`, `"from":"colonized"`)
	if err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"state":"colonized"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-11T09:00:00Z", "colonyStrength": 3, "broodPattern": "solid",
		"temper": "calm", "miteCount": 1, "note": "",
	}, 201, `"inspection-000002"`); err != nil {
		return err
	}
	confirmation, err := confirmationID(pending)
	if err != nil {
		return err
	}
	// 必须由另一个角色（值守人）确认；批准后退役才真正写入。
	if _, err := c.expect("POST", decidePath(confirmation), decisionRequest("keeper-zhou", "approve"), 200,
		`"status":"approved"`, `"to":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"state":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-12T09:00:00Z", "colonyStrength": 3, "broodPattern": "solid",
		"temper": "calm", "miteCount": 1, "note": "",
	}, 409, `"code":"invalid_state"`); err != nil {
		return err
	}
	return nil
}

func checkReplaceQueen(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "bred", "introducedAt": "2026-04-02T08:00:00Z", "reason": "aging",
	}, 201, `"queen-000001"`, `"markColor":"white"`, `"state":"laying"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "cells", "introducedAt": "2026-04-20T08:00:00Z", "reason": "aggression",
	}, 201, `"queen-000002"`, `"markColor":"white"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01/queens", nil, 200,
		`"queen-000001"`, `"superseded"`, `"queen-000002"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "imported", "introducedAt": "2026-05-01T08:00:00Z", "reason": "aging",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	// 退役需要协调人发起、值守人确认后才会生效，生效后换王被拒绝。
	pending, err := c.expect("POST", "/hives/hive-01/state", retireRequest("coord-li"), 202,
		`"status":"pending"`)
	if err != nil {
		return err
	}
	confirmation, err := confirmationID(pending)
	if err != nil {
		return err
	}
	if _, err := c.expect("POST", decidePath(confirmation), decisionRequest("keeper-zhou", "approve"), 200,
		`"status":"approved"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "bred", "introducedAt": "2026-05-02T08:00:00Z", "reason": "loss",
	}, 409, `"code":"invalid_state"`); err != nil {
		return err
	}
	return nil
}

func checkAdvanceHiveState(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	// 低风险迁移仍即时生效，但必须由协调人携带身份发起。
	if _, err := c.expect("POST", "/hives/hive-01/state",
		map[string]any{"target": "production", "requestedBy": "coord-li", "role": "coordinator"}, 200,
		`"from":"colonized"`, `"to":"production"`, `"pending":false`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state",
		map[string]any{"target": "colonized", "requestedBy": "coord-li", "role": "coordinator"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state",
		map[string]any{"target": "production", "requestedBy": "coord-li", "role": "coordinator"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	// 缺少发起身份的请求在校验层被拒绝，不会形成任何事项。
	if _, err := c.expect("POST", "/hives/hive-01/state",
		map[string]any{"target": "retired", "role": "coordinator"}, 400,
		`"code":"invalid_request"`); err != nil {
		return err
	}
	// 值守人不能发起状态推进。
	if _, err := c.expect("POST", "/hives/hive-01/state",
		map[string]any{"target": "retired", "requestedBy": "keeper-zhou", "role": "keeper"}, 400,
		`"code":"invalid_request"`); err != nil {
		return err
	}
	// 退役是高风险目标：协调人发起后只形成待确认事项，蜂箱仍处于 production。
	pending, err := c.expect("POST", "/hives/hive-01/state", retireRequest("coord-li"), 202,
		`"pending":true`, `"status":"pending"`, `"to":"retired"`)
	if err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"state":"production"`); err != nil {
		return err
	}
	confirmation, err := confirmationID(pending)
	if err != nil {
		return err
	}
	// 已有待确认事项时，重复发起被拒绝。
	if _, err := c.expect("POST", "/hives/hive-01/state", retireRequest("coord-li"), 409,
		`"code":"conflict"`); err != nil {
		return err
	}
	// 发起人自己不能确认；协调人角色也不能确认。
	if _, err := c.expect("POST", decidePath(confirmation), decisionRequest("coord-li", "approve"), 409,
		`"code":"conflict"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", decidePath(confirmation),
		map[string]any{"confirmedBy": "coord-wang", "role": "coordinator", "decision": "approve"}, 400,
		`"code":"invalid_request"`); err != nil {
		return err
	}
	// 值守人批准后退役才写入，且只能确认一次。
	if _, err := c.expect("POST", decidePath(confirmation), decisionRequest("keeper-zhou", "approve"), 200,
		`"status":"approved"`, `"to":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"state":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", decidePath(confirmation), decisionRequest("keeper-zhou", "reject"), 409,
		`"code":"conflict"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state",
		map[string]any{"target": "production", "requestedBy": "coord-li", "role": "coordinator"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	// 确认历史与蜂箱状态来自同一份快照：批准记录与实际迁移一致。
	history, err := c.expect("GET", "/hives/hive-01/state/confirmations", nil, 200,
		`"status":"approved"`, `"to":"retired"`, `"confirmedBy":"keeper-zhou"`,
		`"from":"production"`)
	if err != nil {
		return err
	}
	if strings.Count(history.text(), `"id":"confirmation-`) != 1 {
		return fmt.Errorf("确认历史应只有一条事项：%s", history.text())
	}
	if _, err := c.expect("POST", "/state-confirmations/confirmation-missing/decision",
		decisionRequest("keeper-zhou", "approve"), 404, `"code":"not_found"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-missing/state",
		map[string]any{"target": "production", "requestedBy": "coord-li", "role": "coordinator"}, 404,
		`"code":"not_found"`); err != nil {
		return err
	}
	return nil
}

// checkHiveStateConfirmation 覆盖拒绝路径、与巡检/换王交错以及并发确认。
func checkHiveStateConfirmation(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}

	// 拒绝路径：被拒绝的事项不产生状态变化，之后仍可正常推进与重新发起退役。
	pending, err := c.expect("POST", "/hives/hive-01/state", retireRequest("coord-li"), 202,
		`"status":"pending"`)
	if err != nil {
		return err
	}
	firstConfirmation, err := confirmationID(pending)
	if err != nil {
		return err
	}
	if _, err := c.expect("POST", decidePath(firstConfirmation),
		map[string]any{
			"confirmedBy": "keeper-zhou", "role": "keeper", "decision": "reject",
			"reason": "现场蜂群状态不允许退役",
		}, 200,
		`"status":"rejected"`, `"reason":"现场蜂群状态不允许退役"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"state":"colonized"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01/state/confirmations", nil, 200,
		`"status":"rejected"`, `"confirmedBy":"keeper-zhou"`); err != nil {
		return err
	}
	// 拒绝后事项终结，再次操作同一事项被拒绝。
	if _, err := c.expect("POST", decidePath(firstConfirmation),
		decisionRequest("keeper-zhou", "approve"), 409, `"code":"conflict"`); err != nil {
		return err
	}
	// 蜂箱仍可进行低风险推进。
	if _, err := c.expect("POST", "/hives/hive-01/state",
		map[string]any{"target": "production", "requestedBy": "coord-li", "role": "coordinator"}, 200,
		`"to":"production"`); err != nil {
		return err
	}

	// 待确认期间允许巡检与换王（蜂箱尚未退役）；批准时按当前状态复核迁移。
	pending, err = c.expect("POST", "/hives/hive-01/state", retireRequest("coord-li"), 202,
		`"status":"pending"`, `"from":"production"`)
	if err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 4, "broodPattern": "solid",
		"temper": "calm", "miteCount": 2, "note": "待确认期间巡检",
	}, 201, `"inspection-000001"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "bred", "introducedAt": "2026-04-02T08:00:00Z", "reason": "aging",
	}, 201, `"queen-000001"`, `"state":"laying"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"state":"production"`); err != nil {
		return err
	}
	secondConfirmation, err := confirmationID(pending)
	if err != nil {
		return err
	}

	// 并发确认：同一个待确认事项同时收到两个批准，只能有一个成功，状态只能写入一次。
	results := c.callParallel([]parallelCall{
		{method: "POST", path: decidePath(secondConfirmation), payload: decisionRequest("keeper-zhou", "approve")},
		{method: "POST", path: decidePath(secondConfirmation), payload: decisionRequest("keeper-chen", "approve")},
	})
	approved, conflicts := 0, 0
	for _, result := range results {
		if result.err != nil {
			return result.err
		}
		switch result.reply.status {
		case 200:
			if !strings.Contains(result.reply.text(), `"status":"approved"`) {
				return fmt.Errorf("成功的确认响应缺少 approved：%s", result.reply.text())
			}
			approved++
		case 409:
			conflicts++
		default:
			return fmt.Errorf("并发确认出现非预期状态 %d：%s", result.reply.status, result.reply.text())
		}
	}
	if approved != 1 || conflicts != 1 {
		return fmt.Errorf("并发确认应有 1 个批准、1 个冲突，实际 %d 个批准、%d 个冲突", approved, conflicts)
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"state":"retired"`); err != nil {
		return err
	}
	// 退役后交错到达的巡检与换王都被拒绝。
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-12T09:00:00Z", "colonyStrength": 4, "broodPattern": "solid",
		"temper": "calm", "miteCount": 2, "note": "",
	}, 409, `"code":"invalid_state"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "cells", "introducedAt": "2026-04-20T08:00:00Z", "reason": "aggression",
	}, 409, `"code":"invalid_state"`); err != nil {
		return err
	}
	// 汇总与历史一致：retired 计数为 1，批准事项携带的迁移终点也是 retired。
	report, err := c.expect("GET", "/apiaries/apiary-north/status", nil, 200,
		`"hiveCount":1`, `"retired":1`)
	if err != nil {
		return err
	}
	if !strings.Contains(report.text(), `"state":"retired"`) {
		return fmt.Errorf("状态汇总与蜂箱状态不一致：%s", report.text())
	}
	history, err := c.expect("GET", "/hives/hive-01/state/confirmations", nil, 200,
		`"status":"rejected"`, `"status":"approved"`)
	if err != nil {
		return err
	}
	if strings.Count(history.text(), `"status":"approved"`) != 1 {
		return fmt.Errorf("确认历史中批准记录应恰好一条：%s", history.text())
	}
	return nil
}

func checkReadApiaryStatus(c *client) error {
	if err := seedApiary(c, demoApiaryID, "北岭蜂场", "北岭片区", "周岚"); err != nil {
		return err
	}
	for _, hiveID := range []string{"hive-01", "hive-02", "hive-03"} {
		if err := seedHive(c, demoApiaryID, hiveID, "langstroth"); err != nil {
			return err
		}
	}
	if _, err := c.expect("POST", "/hives/hive-02/state",
		map[string]any{"target": "production", "requestedBy": "coord-li", "role": "coordinator"}, 200,
		`"to":"production"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 4, "broodPattern": "solid",
		"temper": "calm", "miteCount": 12, "note": "",
	}, 201, `"mite_load"`); err != nil {
		return err
	}
	report, err := c.expect("GET", "/apiaries/apiary-north/status?asOf=2026-04-12T00:00:00Z", nil, 200,
		`"hiveCount":3`, `"colonized":2`, `"production":1`, `"layingQueens":0`)
	if err != nil {
		return err
	}
	for _, fragment := range []string{
		`"needsInspection":["hive-02","hive-03"]`,
		`"concernHives":["hive-01"]`,
	} {
		if !strings.Contains(report.text(), fragment) {
			return fmt.Errorf("状态汇总缺少 %q：%s", fragment, report.text())
		}
	}
	if err := report.mustOrder(`"hiveId":"hive-01"`, `"hiveId":"hive-02"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/apiaries/apiary-missing/status", nil, 404, `"code":"not_found"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/apiaries/apiary-north/status?asOf=not-a-time", nil, 400,
		`"code":"invalid_request"`); err != nil {
		return err
	}
	return nil
}
