package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/joho/godotenv"
	"github.com/naiba/nezha/cmd/dashboard/controller"
	"github.com/naiba/nezha/cmd/dashboard/rpc"
	"github.com/naiba/nezha/model"
	"github.com/naiba/nezha/proto"
	"github.com/naiba/nezha/service/singleton"
	"github.com/ory/graceful"
	flag "github.com/spf13/pflag"
)

type DashboardCliParam struct {
	Version               bool   // 当前版本号
	ConfigFile            string // 配置文件路径
	DatabaseDriver        string // 数据库驱动
	DatabaseDsn           string // 数据库DSN
	ElasticsearchEnable   string // 是否开启Elasticsearch
	ElasticsearchHosts    string // ES地址
	ElasticsearchUsername string // ES用户名
	ElasticsearchPassword string // ES密码

}

var (
	dashboardCliParam DashboardCliParam
)

func init() {
	flag.CommandLine.ParseErrorsWhitelist.UnknownFlags = true
	flag.BoolVarP(&dashboardCliParam.Version, "version", "v", false, "查看当前版本号")
	flag.StringVarP(&dashboardCliParam.ConfigFile, "config", "c", "data/config.yaml", "配置文件路径")
	flag.StringVarP(&dashboardCliParam.DatabaseDriver, "driver", "d", "", "数据库驱动")
	flag.StringVarP(&dashboardCliParam.DatabaseDsn, "dsn", "s", "", "数据库DSN")
	flag.StringVarP(&dashboardCliParam.ElasticsearchEnable, "es-enable", "e", "", "是否开启Elasticsearch")
	flag.StringVarP(&dashboardCliParam.ElasticsearchHosts, "es-hosts", "h", "", "Elasticsearch地址")
	flag.StringVarP(&dashboardCliParam.ElasticsearchUsername, "es-username", "u", "", "Elasticsearch用户名")
	flag.StringVarP(&dashboardCliParam.ElasticsearchPassword, "es-password", "p", "", "Elasticsearch密码")
	flag.Parse()

	// 加载.env文件
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found")
	}

	// 优先使用命令行参数，如果为空则使用环境变量，最后使用默认值
	if dashboardCliParam.DatabaseDriver == "" {
		dashboardCliParam.DatabaseDriver = getEnvStr("DATABASE_DRIVER", "sqlite")
	}

	if dashboardCliParam.DatabaseDsn == "" {
		dashboardCliParam.DatabaseDsn = getEnvStr("DATABASE_DSN", "data/sqlite.db")
	}

	if dashboardCliParam.ElasticsearchEnable == "" {
		dashboardCliParam.ElasticsearchEnable = getEnvStr("ELASTICSEARCH_ENABLE", "false")
	}

	if dashboardCliParam.ElasticsearchHosts == "" {
		dashboardCliParam.ElasticsearchHosts = getEnvStr("ELASTICSEARCH_HOSTS", "http://localhost:9200")
	}

	if dashboardCliParam.ElasticsearchUsername == "" {
		dashboardCliParam.ElasticsearchUsername = getEnvStr("ELASTICSEARCH_USERNAME", "elastic")
	}

	if dashboardCliParam.ElasticsearchPassword == "" {
		dashboardCliParam.ElasticsearchPassword = getEnvStr("ELASTICSEARCH_PASSWORD", "elastic")
	}

}

// getEnvStr 返回第一个存在的环境变量的值，如果都不存在，则返回 defaultValue。
func getEnvStr(key, defaultValue string) string {
	// 用 "|" 分割 key 字符串，处理多个环境变量名。
	keys := strings.Split(key, "|")
	// 遍历所有的键名。
	for _, k := range keys {
		// 检查环境变量是否存在。
		if value, exists := os.LookupEnv(k); exists {
			// 如果找到，返回环境变量的值。
			return value
		}
	}
	// 如果所有环境变量都不存在，返回默认值。
	return defaultValue
}

func initSystem() {
	// 启动 singleton 包下的所有服务
	singleton.LoadSingleton()

	// 每天的3:30 对 监控记录 和 流量记录 进行清理
	if _, err := singleton.Cron.AddFunc("0 30 3 * * *", singleton.CleanMonitorHistory); err != nil {
		panic(err)
	}

	// 每小时对流量记录进行打点
	if _, err := singleton.Cron.AddFunc("0 0 * * * *", singleton.RecordTransferHourlyUsage); err != nil {
		panic(err)
	}
}

func main() {
	if dashboardCliParam.Version {
		fmt.Println(singleton.Version)
		os.Exit(0)
	}

	// 初始化 dao 包
	singleton.InitConfigFromPath(dashboardCliParam.ConfigFile)
	singleton.InitTimezoneAndCache()
	singleton.InitDB(dashboardCliParam.DatabaseDriver, dashboardCliParam.DatabaseDsn)
	singleton.InitES(dashboardCliParam.ElasticsearchEnable, dashboardCliParam.ElasticsearchHosts, dashboardCliParam.ElasticsearchUsername, dashboardCliParam.ElasticsearchPassword)
	singleton.InitLocalizer()
	initSystem()

	// TODO 使用 cmux 在同一端口服务 HTTP 和 gRPC
	singleton.CleanMonitorHistory()
	go rpc.ServeRPC(singleton.Conf.GRPCPort)
	serviceSentinelDispatchBus := make(chan model.Monitor) // 用于传递服务监控任务信息的channel
	go rpc.DispatchTask(serviceSentinelDispatchBus)
	go rpc.DispatchKeepalive()
	go singleton.AlertSentinelStart()
	singleton.NewServiceSentinel(serviceSentinelDispatchBus)
	srv := controller.ServeWeb(singleton.Conf.HTTPPort)
	go dispatchReportInfoTask()
	if err := graceful.Graceful(func() error {
		return srv.ListenAndServe()
	}, func(c context.Context) error {
		log.Println("NEZHA>> Graceful::START")
		singleton.RecordTransferHourlyUsage()
		log.Println("NEZHA>> Graceful::END")
		err := srv.Shutdown(c)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		log.Printf("NEZHA>> ERROR: %v", err)
	}
}

func dispatchReportInfoTask() {
	time.Sleep(time.Second * 15)
	singleton.ServerLock.RLock()
	defer singleton.ServerLock.RUnlock()
	for _, server := range singleton.ServerList {
		if server == nil || server.TaskStream == nil {
			continue
		}
		err := server.TaskStream.Send(&proto.Task{
			Type: model.TaskTypeReportHostInfo,
			Data: "",
		})
		if err != nil {
			return
		}
	}
}
