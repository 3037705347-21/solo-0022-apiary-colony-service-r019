# 蜂场蜂群协作服务

## 项目目标

为蜂场值守人员和片区协调人提供一个轻量后端服务，用于录入蜂场与蜂箱、提交一次巡检、
更换蜂王、推进蜂群状态，并读取单个蜂场的状态汇总。服务只处理蜂场内部的蜂群协作信息，
数据保存在进程内存中。

## 使用者

- 蜂场值守人负责录入蜂场与蜂箱，并按现场情况提交巡检。
- 片区协调人负责推进蜂箱状态、更换蜂王，并查看蜂场状态汇总。

## 核心实体

- 蜂场（apiary）：编号、名称、片区、值守人、录入时间。
- 蜂箱（hive）：编号、所属蜂场、箱型、蜂群状态、立群时间、当前蜂王编号、最近巡检时间。
- 巡检（inspection）：所属蜂箱、观察时间、群势、子脾形态、性情、螨计数、备注与派生的关注项。
- 蜂王（queen）：编号、所属蜂箱、引入时间、来源、标记色、产卵状态。

## 业务流程

### 录入蜂场（register-apiary）

片区协调人向 `POST /apiaries` 提交蜂场编号、名称、片区和值守人。服务校验编号格式、
名称非空与编号唯一，成功后建立蜂场；编号重复时返回冲突结果。写入成功后可以通过
`GET /apiaries/{apiaryId}` 读回同一份内容。

### 安装蜂箱（install-hive）

值守人向 `POST /apiaries/{apiaryId}/hives` 提交蜂箱编号与箱型。服务先确认蜂场存在，
再校验箱型属于允许集合，并确认蜂箱编号全局唯一；成功后建立状态为「已立群」的蜂箱。
蜂场不存在返回未找到，编号重复返回冲突。

### 提交巡检（record-inspection）

值守人向 `POST /hives/{hiveId}/inspections` 提交群势、子脾形态、性情、螨计数与备注。
服务确认蜂箱存在且未退役，再校验各量测范围，保存巡检记录，并按固定阈值派生关注项。
处于已退役状态的蜂箱返回状态错误，量测越界返回校验错误。

### 更换蜂王（replace-queen）

协调人向 `POST /hives/{hiveId}/queens` 提交蜂王来源、引入时间与原因。服务确认蜂箱存在
且未退役，关闭原有产卵蜂王记录，建立新的蜂王记录并写入由引入年份推导的标记色。
已退役蜂箱返回状态错误。

### 推进蜂群状态（advance-hive-state）

协调人向 `POST /hives/{hiveId}/state` 提交目标状态。服务读取当前状态，按允许的迁移表
判断迁移是否合法；普通目标合法时立即写入新状态并返回迁移前后状态；非法迁移返回状态错误。
推进到高风险目标 `retired` 时，请求方必须声明提交角色（`keeper` 或 `coordinator`），
服务只登记一条待确认事项而不改变蜂箱状态，同一蜂箱同时最多存在一条待确认事项。

### 确认高风险状态迁移（confirm-hive-state）

另一角色向 `POST /hives/{hiveId}/state-confirmations/{confirmationId}` 提交
`confirm` 或 `reject` 决定。确认人角色必须与提交人不同；确认通过时在同一临界区内
重新校验迁移并写入新状态，拒绝时不留下任何状态变化。每条确认事项只能处理一次，
处理结果可通过 `GET /hives/{hiveId}/state-confirmations` 按提交顺序读回，
与蜂箱状态读取保持一致。

### 读取蜂场状态汇总（read-apiary-status）

协调人向 `GET /apiaries/{apiaryId}/status` 查询该蜂场的状态汇总。服务在同一份快照上
聚合蜂箱数量、各状态计数、待巡检蜂箱、有未处理关注项的蜂箱以及产卵蜂王计数，并按蜂箱
编号稳定排序返回每个蜂箱的最新摘要。可选 `asOf` 参数使待巡检判定可复现。

## 状态与规则

### 蜂群状态机

- 状态：`colonized`（已立群）、`production`（产蜜期）、`wintering`（越冬期）、`retired`（已退役）。
- 允许迁移：`colonized->production`、`production->wintering`、`wintering->production`、
  `colonized->retired`、`production->retired`、`wintering->retired`。
- 不变量：`retired` 是终态；同一状态之间的迁移视为冲突；已退役蜂箱不再接受巡检与换王。

### 高风险迁移确认

- 推进到 `retired` 属于高风险迁移，需要 `keeper` 与 `coordinator` 两个角色先后参与：
  一方提交形成待确认事项，另一方确认后才生效。
- 待确认事项状态：`pending`（待确认）、`confirmed`（已确认）、`rejected`（已拒绝）。
- 不变量：未确认前蜂箱状态不变；每条事项只能处理一次；确认人与提交人角色必须不同；
  被拒绝的事项不留下状态变化；确认记录与蜂箱状态在同一临界区写入，读取始终一致。

### 蜂王生命周期

- 状态：`laying`（正常产卵）、`superseded`（被替换）、`lost`（失王）。
- 允许迁移：`laying->superseded`、`laying->lost`。
- 不变量：同一蜂箱同时最多只有一只 `laying` 蜂王；`superseded` 与 `lost` 记录不可再改。
- 标记色由引入年份个位推导：`1/6` 白、`2/7` 黄、`3/8` 红、`4/9` 绿、`5/0` 蓝。

### 巡检规则

- 群势取值 `1..5` 的整数。
- 子脾形态取值 `solid`、`patchy`、`none`。
- 性情取值 `calm`、`nervous`、`aggressive`。
- 螨计数取值 `0..60`，表示每百只成蜂的螨数量。
- 备注长度不超过 200 个字符。

### 关注项派生规则

- `mite_load`：螨计数大于等于 9。
- `brood_gap`：子脾形态为 `none`。
- `temper_risk`：性情为 `aggressive`。
- `low_strength`：群势小于等于 2。

### 状态汇总规则

- 待巡检：从未巡检，或距 `asOf` 超过 14 天。
- 有未处理关注项：该蜂箱最近一次巡检派生出至少一个关注项。
- 待确认状态迁移：该蜂箱存在 `pending` 状态的状态确认事项，与状态计数来自同一份快照。
- 产卵蜂王计数：该蜂场全部蜂箱中处于 `laying` 的蜂王数量。
- `asOf` 缺省取服务当前时间；显式传入时按传入时刻计算，保证结果可复现。

## 模块划分

- `internal/domain`：实体、枚举、校验、领域错误与状态机迁移表。
- `internal/store`：并发安全的内存仓储与一致性快照。
- `internal/apiary`：蜂场与蜂箱录入、蜂箱状态迁移与高风险迁移确认。
- `internal/inspection`：巡检接收与关注项判定。
- `internal/queen`：蜂王更换与标记色推导。
- `internal/status`：蜂场状态汇总聚合。
- `internal/httpapi`：HTTP 路由、请求解析与响应映射。
- `internal/app`：应用组装与配置。
- `cmd/server`：服务进程入口。
- `cmd/check`：按 workflow 执行真实 HTTP 路径的生产校验入口。

## 接口

- `GET /healthz`：返回服务可用状态。
- `POST /apiaries`、`GET /apiaries/{apiaryId}`：录入与读回蜂场。
- `POST /apiaries/{apiaryId}/hives`、`GET /hives/{hiveId}`：安装与读回蜂箱。
- `POST /hives/{hiveId}/inspections`、`GET /hives/{hiveId}/inspections`：提交与列出巡检。
- `POST /hives/{hiveId}/queens`：更换蜂王。
- `POST /hives/{hiveId}/state`：推进蜂群状态；高风险目标返回 202 与待确认事项。
- `POST /hives/{hiveId}/state-confirmations/{confirmationId}`：确认或拒绝待确认事项。
- `GET /hives/{hiveId}/state-confirmations`：列出状态确认历史。
- `GET /apiaries/{apiaryId}/status`：读取蜂场状态汇总。

## 校验计划

- 构建：`go build ./...`。
- 每个 workflow 通过 `go run ./cmd/check -workflow <id>` 走真实 HTTP 入口完成一次端到端校验。
- 校验入口覆盖成功路径与至少一个失败路径（重复编号、未知对象、非法状态迁移或越界量测）。

## 测试边界

本项目按初始化基线交付，**测试刻意推迟**：不包含单元测试、测试夹具或浏览器端校验文件，
`.project-runtime.json` 也不声明 `test_command`。后续工程任务阶段会在保留现有行为的前提下
补充红绿验证用例；当前以 workflow 校验入口作为可执行的生产路径证明。
