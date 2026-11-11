package main

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	dataMu     sync.RWMutex
	dataMap    = map[string]*DataSet{}
	readOnce   bool
	httpClient = &http.Client{Timeout: 20 * time.Second}
)

var defaultTypes = []string{"2", "3", "1", "5", "102", "103"}
var serverTimeZones = map[string]int{"prod_gf_cn": 8, "prod_gf_jp": 8, "prod_gf_us": -5, "prod_gf_eu": 1, "prod_gf_sg": 8}

const (
	apiCN = "https://public-operation-nap.mihoyo.com"
	apiOS = "https://public-operation-nap-sg.hoyoverse.com"
)

type apiResponse struct {
	Retcode int    `json:"retcode"`
	Message string `json:"message"`
	Data    *struct {
		List   []Log  `json:"list"`
		Region string `json:"region"`
	} `json:"data"`
}

func dataFile(uid string) string { return filepath.Join(dataDir, "gacha-list-"+uid+".json") }

func readData() {
	dataMu.Lock()
	defer dataMu.Unlock()
	if readOnce {
		return
	}
	readOnce = true
	files, _ := filepath.Glob(filepath.Join(dataDir, "gacha-list-*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var j DataSetJSON
		if json.Unmarshal(b, &j) != nil || j.UID == "" {
			continue
		}
		ds := fromJSON(j)
		if len(ds.TypeMap) == 0 {
			ds.TypeMap = [][2]string{{"2", "独家频段"}, {"3", "音擎频段"}, {"1", "常驻频段"}, {"5", "邦布频段"}, {"102", "独家重映"}, {"103", "音擎回响"}}
		}
		for _, logs := range ds.Result {
			for i := range logs {
				if logs[i].Count == "" {
					logs[i].Count = "1"
				}
			}
		}
		dataMap[ds.UID] = ds
	}
	cur := currentUID()
	if cur == "" || cur == "0" {
		if len(dataMap) > 0 {
			for uid := range dataMap {
				_ = changeCurrent(uid)
				break
			}
		}
	} else if _, ok := dataMap[cur]; !ok && len(dataMap) > 0 {
		for uid := range dataMap {
			_ = changeCurrent(uid)
			break
		}
	}
}

func saveData(ds *DataSet) error {
	b, err := json.MarshalIndent(ds.toJSON(), "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(dataFile(ds.UID), b, 0644); err != nil {
		return err
	}
	return saveConfig()
}

func changeCurrent(uid string) error { return setConfigValueAndSave("current", uid) }

func setConfigValueAndSave(key string, value interface{}) error {
	if !setConfigValue(key, value) {
		return fmt.Errorf("invalid config")
	}
	return saveConfig()
}

func deleteData(uid string, deleted bool) error {
	dataMu.Lock()
	ds, ok := dataMap[uid]
	if ok {
		ds.Deleted = deleted
	}
	dataMu.Unlock()
	if !ok {
		return nil
	}
	return saveData(ds)
}

func serializeData() map[string]interface{} {
	dataMu.RLock()
	defer dataMu.RUnlock()
	list := make([]DataSetJSON, 0, len(dataMap))
	for _, ds := range dataMap {
		list = append(list, ds.toJSON())
	}
	return map[string]interface{}{"current": getConfig().Current, "data": list}
}

func getCurrentData() *DataSet {
	dataMu.RLock()
	defer dataMu.RUnlock()
	return dataMap[currentUID()]
}

func mergeList(a, b []Log) []Log {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	list := append(append([]Log{}, b...), a...)
	seen := map[string]bool{}
	out := []Log{}
	for _, it := range list {
		if !seen[it.ID] {
			out = append(out, it)
		}
		seen[it.ID] = true
	}
	sort.SliceStable(out, func(i, j int) bool { return cmpID(out[i].ID, out[j].ID) < 0 })
	return out
}

func cmpID(a, b string) int {
	x, ok1 := new(big.Int).SetString(a, 10)
	y, ok2 := new(big.Int).SetString(b, 10)
	if ok1 && ok2 {
		return x.Cmp(y)
	}
	return strings.Compare(a, b)
}

func mergeDataSet(local, origin *DataSet) map[string][]Log {
	if local == nil || local.Result == nil {
		return origin.Result
	}
	if local.UID != origin.UID {
		return origin.Result
	}
	out := map[string][]Log{}
	keys := map[string]bool{}
	for k := range local.Result {
		keys[k] = true
	}
	for k := range origin.Result {
		keys[k] = true
	}
	for k := range keys {
		out[k] = mergeList(local.Result[k], origin.Result[k])
	}
	return out
}

func i18nLog(key string, vars map[string]string) string {
	tpl := i18nText(getConfig().Lang, key)
	for k, v := range vars {
		tpl = strings.ReplaceAll(tpl, "${"+k+"}", v)
	}
	return tpl
}

func fixAuthkey(raw string) string {
	re := regexp.MustCompile(`authkey=([^&]+)`)
	m := re.FindStringSubmatch(raw)
	if len(m) == 2 && strings.Contains(m[1], "=") && !strings.Contains(m[1], "%") {
		return strings.Replace(raw, "authkey="+m[1], "authkey="+url.QueryEscape(m[1]), 1)
	}
	return raw
}

func parseGachaURL(raw string) (url.Values, string, error) {
	u, err := url.Parse(fixAuthkey(raw))
	if err != nil {
		return nil, "", err
	}
	base := apiCN
	if strings.Contains(u.Host, "webstatic-sea") || strings.Contains(u.Host, "hoyoverse.com") {
		base = apiOS
	}
	q := u.Query()
	if q.Get("authkey") == "" {
		return nil, "", fmt.Errorf("%s", i18nText(getConfig().Lang, "log.url.lackAuth"))
	}
	q.Del("page")
	q.Del("size")
	q.Del("real_gacha_type")
	q.Del("end_id")
	return q, base, nil
}

func doGet(rawurl string) (*apiResponse, error) {
	resp, err := httpClient.Get(rawurl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var ar apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, err
	}
	return &ar, nil
}

func checkStatus(res *apiResponse) error {
	if res == nil {
		return fmt.Errorf("empty response")
	}
	if res.Retcode != 0 {
		msg := res.Message
		if msg == "authkey timeout" {
			msg = i18nText(getConfig().Lang, "log.fetch.authTimeout")
			hub.sendMsg("true", "AUTHKEY_TIMEOUT")
		}
		hub.sendMsg(msg, "ERROR")
		return fmt.Errorf("%s", msg)
	}
	hub.sendMsg("false", "AUTHKEY_TIMEOUT")
	return nil
}

func tryRequest(raw string) (string, error) {
	q, base, err := parseGachaURL(raw)
	if err != nil {
		return "", err
	}
	u := base + "/common/gacha_record/api/getGachaLog?" + q.Encode() + "&page=1&size=5&real_gacha_type=1&end_id=0"
	res, err := doGet(u)
	if err != nil {
		return "", err
	}
	if err := checkStatus(res); err != nil {
		return "", err
	}
	return base, nil
}

func tryGetUID(base string, q url.Values) string {
	for _, typ := range defaultTypes {
		u := base + "/common/gacha_record/api/getGachaLog?" + q.Encode() + "&real_gacha_type=" + typ + "&page=1&size=6"
		res, err := doGet(u)
		if err == nil && res.Data != nil && len(res.Data.List) > 0 {
			return res.Data.List[0].UID
		}
	}
	return currentUID()
}

func getGachaLogPage(base, qs string, key string, page, retry int, name, endID string) (*apiResponse, error) {
	u := fmt.Sprintf("%s/common/gacha_record/api/getGachaLog?%s&real_gacha_type=%s&page=%d&size=20", base, qs, key, page)
	if endID != "" {
		u += "&end_id=" + url.QueryEscape(endID)
	}
	res, err := doGet(u)
	if err != nil || res == nil || res.Data == nil || res.Data.List == nil {
		if retry > 0 {
			hub.sendMsg(i18nLog("log.fetch.retry", map[string]string{"name": name, "page": strconv.Itoa(page), "count": strconv.Itoa(6 - retry)}), "STATUS")
			time.Sleep(5 * time.Second)
			return getGachaLogPage(base, qs, key, page, retry-1, name, endID)
		}
		hub.sendMsg(i18nLog("log.fetch.retryFailed", map[string]string{"name": name, "page": strconv.Itoa(page)}), "ERROR")
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("fetch failed")
	}
	if res.Retcode != 0 {
		if err := checkStatus(res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func getGachaLogs(base, qs string, typ GachaType) ([]Log, string, string) {
	list := []Log{}
	page := 1
	uid := ""
	region := ""
	endID := "0"
	cfg := getConfig()
	for {
		if page%10 == 0 {
			hub.sendMsg(i18nLog("log.fetch.interval", map[string]string{"name": typ.Name, "page": strconv.Itoa(page)}), "STATUS")
			time.Sleep(time.Second)
		}
		hub.sendMsg(i18nLog("log.fetch.current", map[string]string{"name": typ.Name, "page": strconv.Itoa(page)}), "STATUS")
		res, err := getGachaLogPage(base, qs, typ.Key, page, 5, typ.Name, endID)
		if err != nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
		logs := []Log{}
		if res.Data != nil {
			logs = res.Data.List
			if uid == "" && len(logs) > 0 {
				uid = logs[0].UID
			}
			if region == "" {
				region = res.Data.Region
			}
		}
		list = append(list, logs...)
		page++
		if len(logs) > 0 {
			endID = logs[len(logs)-1].ID
		}
		if !cfg.FetchFullHistory && len(logs) > 0 && uid != "" {
			dataMu.RLock()
			local, ok := dataMap[uid]
			dataMu.RUnlock()
			if ok {
				if arr, ok := local.Result[typ.Key]; ok && len(arr) > 0 {
					localLatest := arr[len(arr)-1].ID
					if localLatest != "" {
						for _, it := range logs {
							if it.ID == localLatest {
								return list, uid, region
							}
						}
					}
				}
			}
		}
		if len(logs) == 0 {
			break
		}
	}
	return list, uid, region
}

func convertTimeZone(s string, from, to int) string {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.UTC)
	if err != nil {
		return s
	}
	utc := t.Add(time.Duration(-from) * time.Hour)
	target := utc.Add(time.Duration(to) * time.Hour)
	return target.Format("2006-01-02 15:04:05")
}

func fetchData(urlOverride string) (map[string]interface{}, error) {
	readData()
	raw := strings.TrimSpace(urlOverride)
	if raw == "" {
		return nil, fmt.Errorf("%s", i18nText(getConfig().Lang, "log.url.notFound2"))
	}
	base, err := tryRequest(raw)
	if err != nil {
		return nil, err
	}
	q, _, err := parseGachaURL(raw)
	if err != nil {
		return nil, err
	}
	vuid := tryGetUID(base, q)
	dataMu.RLock()
	localDs, hasLocal := dataMap[vuid]
	dataMu.RUnlock()
	if hasLocal && localDs.Lang != "" {
		q.Set("lang", localDs.Lang)
	}
	lang := q.Get("lang")
	types := getGachaTypes(detectLocale(lang))
	if len(types) == 0 {
		types = []GachaType{{"2", "独家频段"}, {"3", "音擎频段"}, {"1", "常驻频段"}, {"5", "邦布频段"}, {"102", "独家重映"}, {"103", "音擎回响"}}
	}
	result := map[string][]Log{}
	typeMap := [][2]string{}
	originUID := ""
	localTZ := 0
	haveTZ := false
	qs := q.Encode()
	for _, typ := range types {
		logs, uid, region := getGachaLogs(base, qs, typ)
		regionTZ, ok := serverTimeZones[region]
		if !ok {
			return nil, fmt.Errorf("unsupported server region: %s", region)
		}
		if !haveTZ {
			if hasLocal && localDs.RegionTZ != 0 {
				localTZ = localDs.RegionTZ
			} else {
				localTZ = regionTZ
			}
			haveTZ = true
		}
		for i := range logs {
			logs[i].Time = convertTimeZone(logs[i].Time, regionTZ, localTZ)
			if logs[i].Count == "" {
				logs[i].Count = "1"
			}
		}
		for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
			logs[i], logs[j] = logs[j], logs[i]
		}
		typeMap = append(typeMap, [2]string{typ.Key, typ.Name})
		result[typ.Key] = logs
		if originUID == "" {
			originUID = uid
		}
	}
	ds := &DataSet{UID: originUID, Time: time.Now().UnixMilli(), Lang: lang, RegionTZ: localTZ, TypeMap: typeMap, Result: result}
	if hasLocal {
		ds.Result = mergeDataSet(localDs, ds)
	}
	dataMu.Lock()
	dataMap[originUID] = ds
	dataMu.Unlock()
	_ = changeCurrent(originUID)
	_ = saveData(ds)
	return serializeData(), nil
}
