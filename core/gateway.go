package core

// 网关是对客户提供的入口
// 整合网络连接对外提供服务
// 提供消息路由 开启，重启，停止网络服务组件
// 对客户端提供认证 维持通信 重连等功能
type Gate interface {
	Route()
}

type gate struct {
}

// 新建网关
func newGate() {

}

// 初始化
func (g *gate) init() {

}

// 开启网络组件
func (g *gate) startNetwork() {

}

// 重启网络组件
func (g *gate) restartNetWork() {

}

// 路由
func (g *gate) Route() {

}
