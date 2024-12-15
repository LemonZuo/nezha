package singleton

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/elastic/go-elasticsearch/v8/typedapi/core/closepointintime"
	"github.com/elastic/go-elasticsearch/v8/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
	"github.com/elastic/go-elasticsearch/v8/typedapi/types/enums/sortorder"
	"io"
	"log"
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
	rangeValue, exists := query["range"]

	if exists {
		delete(query, "range")
	} else {
		rangeValue = 1
	}

	rangeDays, ok := rangeValue.(int)
	if !ok {
		rangeDays = 1
	}
	ctx := context.Background()

	pitResponse, err := ES.OpenPointInTime("nezha-monitor-histories").
		KeepAlive("5m").
		Do(ctx)
	if err != nil {
		log.Println(fmt.Sprintf("Failed to open point in time: %v", err))
		res.CommonResponse = CommonResponse{
			Code:    500,
			Message: err.Error(),
		}
		return res
	}
	// 从response中获取PIT
	var closePointInTimeReq closepointintime.Request
	pitResponseBytes, err := io.ReadAll(pitResponse.Body)
	if err != nil {
		log.Println(fmt.Sprintf("Failed to read response body, %v", err))
		res.CommonResponse = CommonResponse{
			Code:    500,
			Message: err.Error(),
		}
		return res
	}
	err = json.Unmarshal(pitResponseBytes, &closePointInTimeReq)
	if err != nil {
		log.Println(fmt.Sprintf("Failed to unmarshal response body, %v", err))
		res.CommonResponse = CommonResponse{
			Code:    500,
			Message: err.Error(),
		}
		return res
	}

	defer func(request *closepointintime.ClosePointInTime, ctx context.Context) {
		_, err := request.Do(ctx)
		if err != nil {
			log.Println(fmt.Sprintf("Failed to close point in time: %v", err))
			res.CommonResponse = CommonResponse{
				Code:    500,
				Message: err.Error(),
			}
		}
	}(ES.ClosePointInTime().Request(&closePointInTimeReq), ctx)

	// 构建查询
	pastTime := time.Now().AddDate(0, 0, -rangeDays)
	timeStr := pastTime.UTC().Format(time.RFC3339)
	dateMath := types.NewDateMathBuilder().DateMath(types.DateMath(timeStr)).Build()
	dateRangeQueryBuilder := types.NewDateRangeQueryBuilder().Gte(dateMath)
	dateRangeQuery := types.NewRangeQueryBuilder().DateRangeQuery(dateRangeQueryBuilder)
	dateRangeQueryContainer := types.NewQueryContainerBuilder().Range(map[types.Field]*types.RangeQueryBuilder{
		"created_at": dateRangeQuery,
	}).Build()

	containers := []types.QueryContainer{dateRangeQueryContainer}

	// 将传入的查询条件转换为ES查询
	for k, v := range query {
		termQuery := types.NewTermQueryBuilder().Value(types.NewFieldValueBuilder().String(fmt.Sprintf("%v", v)))

		queryContainer := types.NewQueryContainerBuilder().
			Term(map[types.Field]*types.TermQueryBuilder{
				types.Field(k): termQuery,
			}).Build()
		containers = append(containers, queryContainer)
	}

	// 构建最终的查询
	finalQuery := types.NewQueryContainerBuilder().Bool(types.NewBoolQueryBuilder().Must(containers)).Build()

	// 构建排序
	sort := types.Sort{
		&types.SortOptions{
			SortOptions: map[types.Field]types.FieldSort{
				"monitor_id": {Order: &sortorder.Asc},
				"created_at": {Order: &sortorder.Asc},
			},
		},
	}

	// 构建基础请求
	batchSize := 10000
	trackTotalHits := types.NewTrackHitsBuilder().Bool(true).Build()
	req := &search.Request{
		Query:          &finalQuery,
		Sort:           &sort,
		TrackTotalHits: &trackTotalHits,
		Size:           &batchSize,
		Pit: &types.PointInTimeReference{
			Id: closePointInTimeReq.Id,
		},
	}

	var searchAfter *types.SortResults

	for {
		if searchAfter != nil {
			req.SearchAfter = searchAfter
		}

		if Conf.Debug {
			requestBody, _ := json.Marshal(req)
			log.Println(fmt.Sprintf("Elasticsearch Request body: %s", string(requestBody)))
		}

		response, err := ES.Search().
			Request(req).
			Do(ctx)

		if err != nil {
			res.CommonResponse = CommonResponse{
				Code:    500,
				Message: err.Error(),
			}
			return res
		}

		// 定义自定义的响应结构
		var esResponse struct {
			Took     int64 `json:"took"`
			TimedOut bool  `json:"timed_out"`
			Shards   struct {
				Total      int `json:"total"`
				Successful int `json:"successful"`
				Skipped    int `json:"skipped"`
				Failed     int `json:"failed"`
			} `json:"_shards"`
			Hits struct {
				Total struct {
					Value    int64  `json:"value"`
					Relation string `json:"relation"`
				} `json:"total"`
				MaxScore interface{} `json:"max_score"`
				Hits     []struct {
					Index  string                 `json:"_index"`
					ID     string                 `json:"_id"`
					Score  interface{}            `json:"_score"`
					Source map[string]interface{} `json:"_source"`
					Sort   []interface{}          `json:"sort"`
				} `json:"hits"`
			} `json:"hits"`
		}

		if Conf.Debug {
			// 先打印响应内容
			bodyBytes, _ := io.ReadAll(response.Body)
			log.Println(fmt.Sprintf("Elasticsearch Response status: %d, body: %s", response.StatusCode, string(bodyBytes)))
			log.Println(fmt.Sprintf("Elasticsearch Response "))

			// 重新创建一个新的 Reader，因为 ReadAll 会消耗掉原来的内容
			response.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// 解析响应
		if err := json.NewDecoder(response.Body).Decode(&esResponse); err != nil {
			res.CommonResponse = CommonResponse{
				Code:    500,
				Message: "Failed to decode response: " + err.Error(),
			}
			return res
		}
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				fmt.Printf("Failed to close response body, %v", err)
				return
			}
		}(response.Body)

		hits := esResponse.Hits.Hits
		if len(hits) == 0 {
			break
		}

		// 处理当前批次的结果
		for _, hit := range hits {
			var history struct {
				MonitorID uint64    `json:"monitor_id"`
				ServerID  uint64    `json:"server_id"`
				CreatedAt time.Time `json:"created_at"`
				AvgDelay  float32   `json:"avg_delay"`
			}

			// 将 source 转换为 JSON 字节数组
			sourceBytes, err := json.Marshal(hit.Source)
			if err != nil {
				log.Println(fmt.Sprintf("Failed to marshal source: %v", err))
				continue
			}

			// 解析为目标结构体
			if err := json.Unmarshal(sourceBytes, &history); err != nil {
				continue
			}

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

		// 获取下一次查询的 search_after 值
		if len(hits) > 0 {
			lastHit := hits[len(hits)-1]
			// 保留所有排序值
			sortValues := make(types.SortResults, len(lastHit.Sort))
			for i, v := range lastHit.Sort {
				switch val := v.(type) {
				case float64:
					// 对于数字类型（时间戳等），保持数字格式
					sortValues[i] = fmt.Sprintf("%.0f", val)
				default:
					// 其他类型直接转字符串
					sortValues[i] = fmt.Sprintf("%v", val)
				}
			}
			searchAfter = &sortValues
		}
	}

	// 按照监控ID排序添加到结果中
	for _, monitorID := range sortedMonitorIDs {
		res.Result = append(res.Result, resultMap[monitorID])
	}

	return res
}
