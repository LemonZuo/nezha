package singleton

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"log"
	"strconv"
	"time"

	"github.com/patrickmn/go-cache"
	"gorm.io/gorm"

	"github.com/naiba/nezha/model"
	"github.com/naiba/nezha/pkg/utils"

	"github.com/elastic/go-elasticsearch/v8"
)

var Version = "debug"

var (
	Conf  *model.Config
	Cache *cache.Cache
	DB    *gorm.DB
	Loc   *time.Location
	ES    *elasticsearch.TypedClient
)

func InitTimezoneAndCache() {
	var err error
	Loc, err = time.LoadLocation(Conf.Location)
	if err != nil {
		panic(err)
	}

	Cache = cache.New(5*time.Minute, 10*time.Minute)
}

// LoadSingleton 加载子服务并执行
func LoadSingleton() {
	loadNotifications() // 加载通知服务
	loadServers()       // 加载服务器列表
	loadCronTasks()     // 加载定时任务
	loadAPI()
	initNAT()
	initDDNS()
}

// InitConfigFromPath 从给出的文件路径中加载配置
func InitConfigFromPath(path string) {
	Conf = &model.Config{}
	err := Conf.Read(path)
	if err != nil {
		panic(err)
	}
}

// InitDB 初始化数据库
func InitDB(driver string, dsn string) {
	var err error

	if "sqlite" == driver {
		DB, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
			CreateBatchSize: 200,
		})
	} else if "mysql" == driver {
		DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
			CreateBatchSize: 200,
		})
	} else if "postgres" == driver {
		DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			CreateBatchSize: 200,
		})
	} else {
		panic("unknown db driver")
	}
	if err != nil {
		panic(err)
	}
	if Conf.Debug {
		DB = DB.Debug()
	}
	err = DB.AutoMigrate(model.Server{}, model.User{},
		model.Notification{}, model.AlertRule{}, model.Monitor{},
		model.MonitorHistory{}, model.Cron{}, model.Transfer{},
		model.ApiToken{}, model.NAT{}, model.DDNSProfile{})
	if err != nil {
		panic(err)
	}
}

// InitES 初始化 ES
func InitES(enable, hosts, username, password string) {
	log.Println("NEZHA>> 初始化 Elasticsearch 连接...")

	var err error
	isEnable, err := strconv.ParseBool(enable)
	if err != nil {
		log.Println(fmt.Sprintf("NEZHA>> 初始化 Elasticsearch 连接失败: %v", err))
		panic("Elasticsearch 配置错误")
	}
	if !isEnable {
		return
	}

	cfg := elasticsearch.Config{
		Addresses: []string{hosts},
		Username:  username,
		Password:  password,
	}
	log.Println(fmt.Sprintf("NEZHA>> Elasticsearch Host: %s", hosts))

	// Create a new client
	ES, err = elasticsearch.NewTypedClient(cfg)
	if ES == nil || err != nil {
		log.Println(fmt.Sprintf("NEZHA>> 初始化 Elasticsearch 连接失败: %v", err))
		panic("初始化 Elasticsearch 连接失败")
	}
}

// RecordTransferHourlyUsage 对流量记录进行打点
func RecordTransferHourlyUsage() {
	ServerLock.Lock()
	defer ServerLock.Unlock()
	now := time.Now()
	nowTrimSeconds := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
	var txs []model.Transfer
	for id, server := range ServerList {
		tx := model.Transfer{
			ServerID: id,
			In:       utils.Uint64SubInt64(server.State.NetInTransfer, server.PrevTransferInSnapshot),
			Out:      utils.Uint64SubInt64(server.State.NetOutTransfer, server.PrevTransferOutSnapshot),
		}
		if tx.In == 0 && tx.Out == 0 {
			continue
		}
		server.PrevTransferInSnapshot = int64(server.State.NetInTransfer)
		server.PrevTransferOutSnapshot = int64(server.State.NetOutTransfer)
		tx.CreatedAt = nowTrimSeconds
		txs = append(txs, tx)
	}
	if len(txs) == 0 {
		return
	}
	log.Println("NEZHA>> Cron 流量统计入库", len(txs), DB.Create(txs).Error)
}

// CleanMonitorHistory 清理无效或过时的 监控记录 和 流量记录
func CleanMonitorHistory() {
	// 清理已被删除的服务器的监控记录与流量记录
	DB.Unscoped().Delete(&model.MonitorHistory{}, "created_at < ? OR monitor_id NOT IN (SELECT `id` FROM monitors)", time.Now().AddDate(0, 0, -30))
	// 由于网络监控记录的数据较多，并且前端仅使用了 1 天的数据
	// 考虑到 sqlite 数据量问题，仅保留一天数据，
	// server_id = 0 的数据会用于/service页面的可用性展示
	DB.Unscoped().Delete(&model.MonitorHistory{}, "(created_at < ? AND server_id != 0) OR monitor_id NOT IN (SELECT `id` FROM monitors)", time.Now().AddDate(0, 0, -1))
	DB.Unscoped().Delete(&model.Transfer{}, "server_id NOT IN (SELECT `id` FROM servers)")
	// 计算可清理流量记录的时长
	var allServerKeep time.Time
	specialServerKeep := make(map[uint64]time.Time)
	var specialServerIDs []uint64
	var alerts []model.AlertRule
	DB.Find(&alerts)
	for _, alert := range alerts {
		for _, rule := range alert.Rules {
			// 是不是流量记录规则
			if !rule.IsTransferDurationRule() {
				continue
			}
			dataCouldRemoveBefore := rule.GetTransferDurationStart().UTC()
			// 判断规则影响的机器范围
			if rule.Cover == model.RuleCoverAll {
				// 更新全局可以清理的数据点
				if allServerKeep.IsZero() || allServerKeep.After(dataCouldRemoveBefore) {
					allServerKeep = dataCouldRemoveBefore
				}
			} else {
				// 更新特定机器可以清理数据点
				for id := range rule.Ignore {
					if specialServerKeep[id].IsZero() || specialServerKeep[id].After(dataCouldRemoveBefore) {
						specialServerKeep[id] = dataCouldRemoveBefore
						specialServerIDs = append(specialServerIDs, id)
					}
				}
			}
		}
	}
	for id, couldRemove := range specialServerKeep {
		DB.Unscoped().Delete(&model.Transfer{}, "server_id = ? AND `created_at` < ?", id, couldRemove)
	}
	if allServerKeep.IsZero() {
		DB.Unscoped().Delete(&model.Transfer{}, "server_id NOT IN (?)", specialServerIDs)
	} else {
		DB.Unscoped().Delete(&model.Transfer{}, "server_id NOT IN (?) AND `created_at` < ?", specialServerIDs, allServerKeep)
	}
}

// IPDesensitize 根据设置选择是否对IP进行打码处理 返回处理后的IP(关闭打码则返回原IP)
func IPDesensitize(ip string) string {
	if Conf.EnablePlainIPInNotification {
		return ip
	}
	return utils.IPDesensitize(ip)
}
