package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"
)

const appName = "zzz-signal-search-export-mygo"
const appVersion = "1.0.1"

func uigfTimeString() string {
	return time.Now().Format("20060102_1504")
}

func exportUIGF(uids []string) ([]byte, string, error) {
	dataMu.RLock()
	accounts := []map[string]interface{}{}
	for _, uid := range uids {
		ds, ok := dataMap[uid]
		if !ok {
			continue
		}
		logs := []Log{}
		for _, arr := range ds.Result {
			logs = append(logs, arr...)
		}
		sort.SliceStable(logs, func(i, j int) bool { return cmpID(logs[i].ID, logs[j].ID) < 0 })
		list := make([]map[string]interface{}, 0, len(logs))
		for _, l := range logs {
			list = append(list, map[string]interface{}{
				"gacha_id": l.GachaID, "gacha_type": l.GachaType, "item_id": l.ItemID,
				"count": l.Count, "time": l.Time, "name": l.Name, "item_type": l.ItemType,
				"rank_type": l.RankType, "id": l.ID,
			})
		}
		accounts = append(accounts, map[string]interface{}{"uid": ds.UID, "timezone": ds.RegionTZ, "lang": ds.Lang, "list": list})
	}
	dataMu.RUnlock()
	if len(accounts) == 0 {
		return nil, "", fmt.Errorf("数据为空 / no data")
	}
	result := map[string]interface{}{
		"info": map[string]interface{}{
			"export_timestamp": time.Now().Unix(), "export_app": appName, "export_app_version": "v" + appVersion, "version": "v4.0",
		},
		"nap": accounts,
	}
	b, err := json.Marshal(result)
	if err != nil {
		return nil, "", err
	}
	name := "UIGF_" + uigfTimeString() + ".json"
	if len(accounts) == 1 {
		name = "UIGF_" + anyToString(accounts[0]["uid"]) + "_" + uigfTimeString() + ".json"
	}
	return b, name, nil
}

func intFromAny(v interface{}) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case string:
		n, _ := strconv.Atoi(x)
		return n
	default:
		return 0
	}
}

func importUIGF(content []byte) error {
	var root map[string]interface{}
	if err := json.Unmarshal(content, &root); err != nil {
		return err
	}
	info, _ := root["info"].(map[string]interface{})
	if info == nil {
		return fmt.Errorf("UIGF格式错误 / invalid UIGF format")
	}
	ver, _ := info["version"].(string)
	if ver != "v4.0" && ver != "v4.1" {
		return fmt.Errorf("不支持此版本UIGF / unsupported UIGF version")
	}
	nap, ok := root["nap"].([]interface{})
	if !ok {
		return fmt.Errorf("UIGF格式错误 / invalid UIGF nap")
	}

	cfg := getConfig()
	for _, item := range nap {
		acc, _ := item.(map[string]interface{})
		if acc == nil {
			continue
		}
		uid := anyToString(acc["uid"])
		if uid == "" {
			continue
		}
		dataMu.RLock()
		existing, exists := dataMap[uid]
		dataMu.RUnlock()
		isNew := !exists
		regionTZ := intFromAny(acc["timezone"])
		targetLang, _ := acc["lang"].(string)
		if !isNew {
			regionTZ = existing.RegionTZ
			targetLang = existing.Lang
		}
		if targetLang == "" {
			targetLang = cfg.Lang
		}
		itemMap := getItemMap(targetLang)
		if itemMap == nil {
			itemMap = getItemMap("zh-cn")
		}
		list, _ := acc["list"].([]interface{})
		result := map[string][]Log{}
		for _, le := range list {
			e, _ := le.(map[string]interface{})
			if e == nil {
				continue
			}
			itemID := anyToString(e["item_id"])
			name := anyToString(e["name"])
			itemType := anyToString(e["item_type"])
			rank := anyToString(e["rank_type"])
			if inf, ok := itemMap[itemID]; ok {
				if inf.Name != "" {
					name = inf.Name
				}
				if inf.ItemType != "" {
					itemType = inf.ItemType
				}
				rank = strconv.Itoa(inf.RankType)
			}
			count := anyToString(e["count"])
			if count == "" {
				count = "1"
			}
			log := Log{
				ID: anyToString(e["id"]), ItemID: itemID, ItemType: itemType, Name: name,
				RankType: rank, Time: convertTimeZone(anyToString(e["time"]), intFromAny(acc["timezone"]), regionTZ),
				GachaID: anyToString(e["gacha_id"]), GachaType: anyToString(e["gacha_type"]), Count: count,
			}
			result[log.GachaType] = append(result[log.GachaType], log)
		}
		ds := &DataSet{UID: uid, Time: time.Now().UnixMilli(), Lang: targetLang, RegionTZ: regionTZ, TypeMap: [][2]string{}}
		if !isNew {
			ds.TypeMap = existing.TypeMap
			ds.Deleted = existing.Deleted
		}
		if len(ds.TypeMap) == 0 {
			for _, typ := range getGachaTypes(detectLocale(targetLang)) {
				ds.TypeMap = append(ds.TypeMap, [2]string{typ.Key, typ.Name})
			}
		}
		ds.Result = result
		if !isNew {
			ds.Result = mergeDataSet(existing, ds)
		}
		dataMu.Lock()
		dataMap[uid] = ds
		dataMu.Unlock()
		_ = saveData(ds)
		_ = changeCurrent(uid)
	}
	return nil
}
