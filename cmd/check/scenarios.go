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

func jsonField(body []byte, key string) (string, error) {
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("响应不是合法 JSON：%v", err)
	}
	value, ok := decoded[key].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("响应缺少字段 %q：%s", key, body)
	}
	return value, nil
}

// requestRetirement 提交高风险退役申请并返回待确认事项编号。
func requestRetirement(c *client, hiveID, requestedBy string) (string, error) {
	reply, err := c.expect("POST", "/hives/"+hiveID+"/state", map[string]any{
		"target": "retired", "requestedBy": requestedBy,
	}, 202, `"state":"pending"`, `"to":"retired"`, `"requestedBy":"`+requestedBy+`"`)
	if err != nil {
		return "", err
	}
	return jsonField(reply.body, "id")
}

// decideConfirmation 由指定角色处理待确认事项。
func decideConfirmation(c *client, hiveID, confirmationID, decision, decidedBy string, wantStatus int, wantFragments ...string) error {
	_, err := c.expect("POST", "/hives/"+hiveID+"/state-confirmations/"+confirmationID, map[string]any{
		"decision": decision, "decidedBy": decidedBy,
	}, wantStatus, wantFragments...)
	return err
}

// retireHive 通过「申请 + 另一角色确认」把蜂箱推进到已退役。
func retireHive(c *client, hiveID string) error {
	confirmationID, err := requestRetirement(c, hiveID, "coordinator")
	if err != nil {
		return err
	}
	if err := decideConfirmation(c, hiveID, confirmationID, "confirm", "keeper", 200,
		`"state":"confirmed"`, `"decidedBy":"keeper"`); err != nil {
		return err
	}
	_, err = c.expect("GET", "/hives/"+hiveID, nil, 200, `"state":"retired"`)
	return err
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
	if err := retireHive(c, "hive-01"); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-11T09:00:00Z", "colonyStrength": 3, "broodPattern": "solid",
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
	if err := retireHive(c, "hive-01"); err != nil {
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
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "production"}, 200,
		`"from":"colonized"`, `"to":"production"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "colonized"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "production"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	// 高风险目标只登记待确认事项，确认前蜂箱状态不变。
	confirmationID, err := requestRetirement(c, "hive-01", "coordinator")
	if err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"state":"production"`); err != nil {
		return err
	}
	if err := decideConfirmation(c, "hive-01", confirmationID, "confirm", "keeper", 200,
		`"state":"confirmed"`, `"to":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"state":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "production"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-missing/state", map[string]any{"target": "production"}, 404,
		`"code":"not_found"`); err != nil {
		return err
	}
	return nil
}

func checkConfirmHiveState(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	// 高风险目标必须声明提交角色，且角色必须受支持。
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "retired"}, 400,
		`"code":"invalid_request"`, `"field":"requestedBy"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{
		"target": "retired", "requestedBy": "stranger",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	// 提交后只形成待确认事项，蜂箱状态不变，巡检与换王照常受理。
	firstID, err := requestRetirement(c, "hive-01", "coordinator")
	if err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"state":"colonized"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 4, "broodPattern": "solid",
		"temper": "calm", "miteCount": 2, "note": "",
	}, 201, `"inspection-000001"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "bred", "introducedAt": "2026-04-02T08:00:00Z", "reason": "aging",
	}, 201, `"queen-000001"`); err != nil {
		return err
	}
	// 待确认事项可以从确认历史与蜂场状态汇总中一致地读到。
	if _, err := c.expect("GET", "/hives/hive-01/state-confirmations", nil, 200,
		firstID, `"state":"pending"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/apiaries/apiary-north/status", nil, 200,
		`"pendingConfirmations":["hive-01"]`, `"colonized":1`); err != nil {
		return err
	}
	// 同一蜂箱已有待确认事项时重复提交返回冲突。
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{
		"target": "retired", "requestedBy": "keeper",
	}, 409, `"code":"conflict"`); err != nil {
		return err
	}
	// 与提交人相同的角色不能确认。
	if err := decideConfirmation(c, "hive-01", firstID, "confirm", "coordinator", 409,
		`"code":"conflict"`); err != nil {
		return err
	}
	// 被拒绝的确认不留下状态变化，且不能再次处理。
	if err := decideConfirmation(c, "hive-01", firstID, "reject", "keeper", 200,
		`"state":"rejected"`, `"decidedBy":"keeper"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"state":"colonized"`); err != nil {
		return err
	}
	if err := decideConfirmation(c, "hive-01", firstID, "confirm", "keeper", 409,
		`"code":"conflict"`); err != nil {
		return err
	}
	// 重新提交后由另一角色确认才生效，且只能确认一次。
	secondID, err := requestRetirement(c, "hive-01", "coordinator")
	if err != nil {
		return err
	}
	if err := decideConfirmation(c, "hive-01", secondID, "confirm", "keeper", 200,
		`"state":"confirmed"`, `"from":"colonized"`, `"to":"retired"`); err != nil {
		return err
	}
	if err := decideConfirmation(c, "hive-01", secondID, "confirm", "keeper", 409,
		`"code":"conflict"`); err != nil {
		return err
	}
	// 确认历史与蜂箱状态读取一致。
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"state":"retired"`); err != nil {
		return err
	}
	history, err := c.expect("GET", "/hives/hive-01/state-confirmations", nil, 200,
		firstID, `"rejected"`, secondID, `"confirmed"`)
	if err != nil {
		return err
	}
	if err := history.mustOrder(firstID, secondID); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/apiaries/apiary-north/status", nil, 200,
		`"retired":1`, `"pendingConfirmations":[]`); err != nil {
		return err
	}
	// 未知对象与非法决定。
	if _, err := c.expect("POST", "/hives/hive-missing/state", map[string]any{
		"target": "retired", "requestedBy": "keeper",
	}, 404, `"code":"not_found"`); err != nil {
		return err
	}
	if err := decideConfirmation(c, "hive-01", "confirmation-999999", "confirm", "keeper", 404,
		`"code":"not_found"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-missing/state-confirmations", nil, 404,
		`"code":"not_found"`); err != nil {
		return err
	}
	if err := decideConfirmation(c, "hive-01", secondID, "maybe", "keeper", 400,
		`"code":"invalid_request"`, `"field":"decision"`); err != nil {
		return err
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
	if _, err := c.expect("POST", "/hives/hive-02/state", map[string]any{"target": "production"}, 200,
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
