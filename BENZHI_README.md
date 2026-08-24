# zonedns

zonedns 是一个权威 DNS 区域管理与解析基础设施：集中维护区域（zone）内的记录，
以 SOA 序列号驱动主备区域传输（AXFR/IXFR），支持 DNSSEC 签名与密钥轮换、
动态更新（DDNS）与变更日志、按规范化域名命中记录的解析查询与应答缓存。
所有状态在进程内维护，HTTP 入口提供查询、更新、传输与运维控制演示。

## 构建

```bash
./build_benzhi_docker.sh zonedns linux/amd64
./build_benzhi_docker.sh zonedns linux/arm64
```

## 运行

```bash
go run ./cmd/zonedns -http 127.0.0.1:18080
go run ./cmd/zonedns -demo
```

## 容器内验证

```bash
go build -mod=vendor ./...
go test -mod=vendor ./...
go vet -mod=vendor ./...
```

## 功能

- 区域仓库与 SOA 序列号版本状态机
- 主备区域传输状态机（idle/in-progress/complete/failed）与 AXFR/IXFR 增量
- DNSSEC 签名（ECDSA P-256）与密钥轮换
- 动态更新：变更日志先落盘、内存回滚、持久化后再通知
- 权威解析查询、负缓存与应答缓存
