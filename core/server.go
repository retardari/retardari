package core

// server 负责整个服务进程的生命周期管理 是整个程序的入口与管理中心
// server init 初始化server
// 启动网关与网络监听与初始化路由
// 启动注册中心
// 注册内部技能
// 注册与启动系统监控
// 提供重启，加载，关闭接口
// 注册信号

type Server struct {
	gate *gate
}

func NewServer() *Server {

	return nil
}

func (s *Server) Start() {

}

func (s *Server) Restart() {

}

func (s *Server) Close() {

}
