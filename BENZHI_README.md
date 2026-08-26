# BENZHI_README

## 项目说明
- 项目：benzhi-project-a022baa0-9876-4160-8fab-42398813b1fe
- 项目用途：冰芯解冻完整性放行服务以版本化 HTTP JSON API 编排样品登记、运输偏差核查、风险分级、方案审批、阶段观测、独立复核与哈希链封存，支持本地快照恢复和请求幂等。
- Go 工具链：`golang:1.22`
- 前端工具链：无

## 项目描述
- 项目名称：冰芯解冻完整性放行服务
- 项目介绍：为极地冰芯样品建立从批次登记、运输偏差核查、完整性分级、解冻方案审批、分阶段解冻记录、独立复核到不可变封存的单一质量放行闭环。
- 项目概述：为极地冰芯样品建立从批次登记、运输偏差核查、完整性分级、解冻方案审批、分阶段解冻记录、独立复核到不可变封存的单一质量放行闭环。
- 核心工作流：样品批次登记→运输温度偏差核查→完整性风险分级→解冻方案审批→分阶段解冻记录→独立复核与异常处置→放行报告封存
- 对外接口：版本化 HTTP JSON API；支持 -addr=127.0.0.1:<port> 或 PORT 环境变量，默认监听 127.0.0.1:19081，提供批次、偏差、方案、解冻观测、复核和封存端点

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/iceguard -self-check -addr=127.0.0.1:19081

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-a022baa0-9876-4160-8fab-42398813b1fe-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-a022baa0-9876-4160-8fab-42398813b1fe-arm64 linux/arm64

docker run -it benzhi-project-a022baa0-9876-4160-8fab-42398813b1fe-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/iceguard -self-check -addr=127.0.0.1:19081`
