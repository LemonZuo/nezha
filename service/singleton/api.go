package singleton

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/naiba/nezha/model"
	"github.com/naiba/nezha/pkg/utils"
)

var (
	ApiTokenList         = make(map[string]*model.ApiToken)
	UserIDToApiTokenList = make(map[uint64][]string)
	ApiLock              sync.RWMutex

	ServerAPI  = &ServerAPIService{}
	MonitorAPI = &MonitorAPIService{}
)

type ServerAPIService struct{}

// CommonResponse 常规返回结构 包含状态码 和 状态信息
type CommonResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type RegisterServer struct {
	Name         string
	Tag          string
	Note         string
	HideForGuest string
}

type ServerRegisterResponse struct {
	CommonResponse
	Secret string `json:"secret"`
}

type CommonServerInfo struct {
	ID           uint64 `json:"id"`
	Name         string `json:"name"`
	Tag          string `json:"tag"`
	LastActive   int64  `json:"last_active"`
	IPV4         string `json:"ipv4"`
	IPV6         string `json:"ipv6"`
	ValidIP      string `json:"valid_ip"`
	DisplayIndex int    `json:"display_index"`
	HideForGuest bool   `json:"hide_for_guest"`
}

// StatusResponse 服务器状态子结构 包含服务器信息与状态信息
type StatusResponse struct {
	CommonServerInfo
	Host   *model.Host      `json:"host"`
	Status *model.HostState `json:"status"`
}

// ServerStatusResponse 服务器状态返回结构 包含常规返回结构 和 服务器状态子结构
type ServerStatusResponse struct {
	CommonResponse
	Result []*StatusResponse `json:"result"`
}

// ServerInfoResponse 服务器信息返回结构 包含常规返回结构 和 服务器信息子结构
type ServerInfoResponse struct {
	CommonResponse
	Result []*CommonServerInfo `json:"result"`
}

type MonitorAPIService struct {
}

type MonitorInfoResponse struct {
	CommonResponse
	Result []*MonitorInfo `json:"result"`
}

type MonitorInfo struct {
	MonitorID   uint64    `json:"monitor_id"`
	ServerID    uint64    `json:"server_id"`
	MonitorName string    `json:"monitor_name"`
	ServerName  string    `json:"server_name"`
	CreatedAt   []int64   `json:"created_at"`
	AvgDelay    []float32 `json:"avg_delay"`
}

func InitAPI() {
	ApiTokenList = make(map[string]*model.ApiToken)
	UserIDToApiTokenList = make(map[uint64][]string)
}

func loadAPI() {
	InitAPI()
	var tokenList []*model.ApiToken
	DB.Find(&tokenList)
	for _, token := range tokenList {
		ApiTokenList[token.Token] = token
		UserIDToApiTokenList[token.UserID] = append(UserIDToApiTokenList[token.UserID], token.Token)
	}
}

// GetStatusByIDList 获取传入IDList的服务器状态信息
func (s *ServerAPIService) GetStatusByIDList(idList []uint64) *ServerStatusResponse {
	res := &ServerStatusResponse{}
	res.Result = make([]*StatusResponse, 0)

	ServerLock.RLock()
	defer ServerLock.RUnlock()

	for _, v := range idList {
		server := ServerList[v]
		if server == nil {
			continue
		}
		ipv4, ipv6, validIP := utils.SplitIPAddr(server.Host.IP)
		info := CommonServerInfo{
			ID:         server.ID,
			Name:       server.Name,
			Tag:        server.Tag,
			LastActive: server.LastActive.Unix(),
			IPV4:       ipv4,
			IPV6:       ipv6,
			ValidIP:    validIP,
		}
		res.Result = append(res.Result, &StatusResponse{
			CommonServerInfo: info,
			Host:             server.Host,
			Status:           server.State,
		})
	}
	res.CommonResponse = CommonResponse{
		Code:    0,
		Message: "success",
	}
	return res
}

// GetStatusByTag 获取传入分组的所有服务器状态信息
func (s *ServerAPIService) GetStatusByTag(tag string) *ServerStatusResponse {
	return s.GetStatusByIDList(ServerTagToIDList[tag])
}

// GetAllStatus 获取所有服务器状态信息
func (s *ServerAPIService) GetAllStatus() *ServerStatusResponse {
	res := &ServerStatusResponse{}
	res.Result = make([]*StatusResponse, 0)
	ServerLock.RLock()
	defer ServerLock.RUnlock()
	for _, v := range ServerList {
		host := v.Host
		state := v.State
		if host == nil || state == nil {
			continue
		}
		ipv4, ipv6, validIP := utils.SplitIPAddr(host.IP)
		info := CommonServerInfo{
			ID:           v.ID,
			Name:         v.Name,
			Tag:          v.Tag,
			LastActive:   v.LastActive.Unix(),
			IPV4:         ipv4,
			IPV6:         ipv6,
			ValidIP:      validIP,
			DisplayIndex: v.DisplayIndex,
			HideForGuest: v.HideForGuest,
		}
		res.Result = append(res.Result, &StatusResponse{
			CommonServerInfo: info,
			Host:             v.Host,
			Status:           v.State,
		})
	}
	res.CommonResponse = CommonResponse{
		Code:    0,
		Message: "success",
	}
	return res
}

// GetListByTag 获取传入分组的所有服务器信息
func (s *ServerAPIService) GetListByTag(tag string) *ServerInfoResponse {
	res := &ServerInfoResponse{}
	res.Result = make([]*CommonServerInfo, 0)

	ServerLock.RLock()
	defer ServerLock.RUnlock()
	for _, v := range ServerTagToIDList[tag] {
		host := ServerList[v].Host
		if host == nil {
			continue
		}
		ipv4, ipv6, validIP := utils.SplitIPAddr(host.IP)
		info := &CommonServerInfo{
			ID:         v,
			Name:       ServerList[v].Name,
			Tag:        ServerList[v].Tag,
			LastActive: ServerList[v].LastActive.Unix(),
			IPV4:       ipv4,
			IPV6:       ipv6,
			ValidIP:    validIP,
		}
		res.Result = append(res.Result, info)
	}
	res.CommonResponse = CommonResponse{
		Code:    0,
		Message: "success",
	}
	return res
}

// GetAllList 获取所有服务器信息
func (s *ServerAPIService) GetAllList() *ServerInfoResponse {
	res := &ServerInfoResponse{}
	res.Result = make([]*CommonServerInfo, 0)

	ServerLock.RLock()
	defer ServerLock.RUnlock()
	for _, v := range ServerList {
		host := v.Host
		if host == nil {
			continue
		}
		ipv4, ipv6, validIP := utils.SplitIPAddr(host.IP)
		info := &CommonServerInfo{
			ID:         v.ID,
			Name:       v.Name,
			Tag:        v.Tag,
			LastActive: v.LastActive.Unix(),
			IPV4:       ipv4,
			IPV6:       ipv6,
			ValidIP:    validIP,
		}
		res.Result = append(res.Result, info)
	}
	res.CommonResponse = CommonResponse{
		Code:    0,
		Message: "success",
	}
	return res
}
func (s *ServerAPIService) Register(rs *RegisterServer) *ServerRegisterResponse {
	var serverInfo model.Server
	var err error
	// Populate serverInfo fields
	serverInfo.Name = rs.Name
	serverInfo.Tag = rs.Tag
	serverInfo.Note = rs.Note
	serverInfo.HideForGuest = rs.HideForGuest == "on"
	// Generate a random secret
	serverInfo.Secret, err = utils.GenerateRandomString(18)
	if err != nil {
		return &ServerRegisterResponse{
			CommonResponse: CommonResponse{
				Code:    500,
				Message: "Generate secret failed: " + err.Error(),
			},
			Secret: "",
		}
	}
	// Attempt to save serverInfo in the database
	err = DB.Create(&serverInfo).Error
	if err != nil {
		return &ServerRegisterResponse{
			CommonResponse: CommonResponse{
				Code:    500,
				Message: "Database error: " + err.Error(),
			},
			Secret: "",
		}
	}

	serverInfo.Host = &model.Host{}
	serverInfo.State = &model.HostState{}
	serverInfo.TaskCloseLock = new(sync.Mutex)
	ServerLock.Lock()
	SecretToID[serverInfo.Secret] = serverInfo.ID
	ServerList[serverInfo.ID] = &serverInfo
	ServerTagToIDList[serverInfo.Tag] = append(ServerTagToIDList[serverInfo.Tag], serverInfo.ID)
	ServerLock.Unlock()
	ReSortServer()
	// Successful response
	return &ServerRegisterResponse{
		CommonResponse: CommonResponse{
			Code:    200,
			Message: "Server created successfully",
		},
		Secret: serverInfo.Secret,
	}
}

// GetMonitorHistories 获取监控历史
func (m *MonitorAPIService) GetMonitorHistories(query map[string]any) *MonitorInfoResponse {
	if ES != nil {
		return m.getMonitorHistoriesFromES(query)
	} else {
		return m.getMonitorHistoriesFromDB(query)
	}
}

// GetMonitorHistoriesFromDB 从数据库获取监控历史
func (m *MonitorAPIService) getMonitorHistoriesFromDB(query map[string]any) *MonitorInfoResponse {
	var (
		resultMap        = make(map[uint64]*MonitorInfo)
		monitorHistories []*model.MonitorHistory
		sortedMonitorIDs []uint64
	)
	res := &MonitorInfoResponse{
		CommonResponse: CommonResponse{
			Code:    0,
			Message: "success",
		},
	}
	if err := DB.Model(&model.MonitorHistory{}).Select("monitor_id, created_at, server_id, avg_delay").
		Where(query).Where("created_at >= ?", time.Now().Add(-24*time.Hour)).Order("monitor_id, created_at").
		Scan(&monitorHistories).Error; err != nil {
		res.CommonResponse = CommonResponse{
			Code:    500,
			Message: err.Error(),
		}
	} else {
		for _, history := range monitorHistories {
			infos, ok := resultMap[history.MonitorID]
			if !ok {
				infos = &MonitorInfo{
					MonitorID:   history.MonitorID,
					ServerID:    history.ServerID,
					MonitorName: ServiceSentinelShared.monitors[history.MonitorID].Name,
					ServerName:  ServerList[history.ServerID].Name,
				}
				resultMap[history.MonitorID] = infos
				sortedMonitorIDs = append(sortedMonitorIDs, history.MonitorID)
			}
			infos.CreatedAt = append(infos.CreatedAt, history.CreatedAt.Truncate(time.Minute).Unix()*1000)
			infos.AvgDelay = append(infos.AvgDelay, history.AvgDelay)
		}
		for _, monitorID := range sortedMonitorIDs {
			res.Result = append(res.Result, resultMap[monitorID])
		}
	}
	return res
}

// GetMonitorHistoriesFromES 从ES获取监控历史
func (m *MonitorAPIService) getMonitorHistoriesFromES(query map[string]any) *MonitorInfoResponse {
	var (
		resultMap        = make(map[uint64]*MonitorInfo)
		sortedMonitorIDs []uint64
	)

	res := &MonitorInfoResponse{
		CommonResponse: CommonResponse{
			Code:    0,
			Message: "success",
		},
	}

	// 构建查询
	must := []map[string]interface{}{
		{
			"range": map[string]interface{}{
				"created_at": map[string]interface{}{
					"gte": time.Now().Add(-24 * time.Hour),
				},
			},
		},
	}

	// 将传入的查询条件转换为ES查询
	for k, v := range query {
		must = append(must, map[string]interface{}{
			"term": map[string]interface{}{
				k: v,
			},
		})
	}

	// 构建完整的查询体
	searchBody := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": must,
			},
		},
		"sort": []map[string]interface{}{
			{"monitor_id": "asc"},
			{"created_at": "asc"},
		},
		"size": 10000,
	}

	// 转换为JSON
	searchJSON, err := json.Marshal(searchBody)
	if err != nil {
		res.CommonResponse = CommonResponse{
			Code:    500,
			Message: err.Error(),
		}
		return res
	}

	// 执行搜索
	response, err := ES.Search(
		ES.Search.WithContext(context.Background()),
		ES.Search.WithIndex("monitor_histories"),
		ES.Search.WithBody(bytes.NewReader(searchJSON)),
	)

	if err != nil {
		res.CommonResponse = CommonResponse{
			Code:    500,
			Message: err.Error(),
		}
		return res
	}
	defer response.Body.Close()

	// 解析响应
	var searchResult struct {
		Hits struct {
			Hits []struct {
				Source struct {
					MonitorID uint64    `json:"monitor_id"`
					ServerID  uint64    `json:"server_id"`
					CreatedAt time.Time `json:"created_at"`
					AvgDelay  float32   `json:"avg_delay"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(response.Body).Decode(&searchResult); err != nil {
		res.CommonResponse = CommonResponse{
			Code:    500,
			Message: err.Error(),
		}
		return res
	}

	// 处理结果
	for _, hit := range searchResult.Hits.Hits {
		history := hit.Source
		infos, ok := resultMap[history.MonitorID]
		if !ok {
			infos = &MonitorInfo{
				MonitorID:   history.MonitorID,
				ServerID:    history.ServerID,
				MonitorName: ServiceSentinelShared.monitors[history.MonitorID].Name,
				ServerName:  ServerList[history.ServerID].Name,
			}
			resultMap[history.MonitorID] = infos
			sortedMonitorIDs = append(sortedMonitorIDs, history.MonitorID)
		}

		infos.CreatedAt = append(infos.CreatedAt, history.CreatedAt.Truncate(time.Minute).Unix()*1000)
		infos.AvgDelay = append(infos.AvgDelay, history.AvgDelay)
	}

	// 按照监控ID排序添加到结果中
	for _, monitorID := range sortedMonitorIDs {
		res.Result = append(res.Result, resultMap[monitorID])
	}

	return res
}
