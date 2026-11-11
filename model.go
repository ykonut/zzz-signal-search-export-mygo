package main

import (
	"encoding/json"
	"sort"
	"strconv"
)

type Log struct {
	ID        string `json:"id"`
	UID       string `json:"uid"`
	ItemID    string `json:"item_id"`
	ItemType  string `json:"item_type"`
	Name      string `json:"name"`
	RankType  string `json:"rank_type"`
	Time      string `json:"time"`
	GachaID   string `json:"gacha_id"`
	GachaType string `json:"gacha_type"`
	Count     string `json:"count"`
}

type DataSet struct {
	UID      string
	Time     int64
	Lang     string
	RegionTZ int
	Deleted  bool
	TypeMap  [][2]string
	Result   map[string][]Log
}

type ResultEntry struct {
	Key  string
	Logs []Log
}
type ResultEntries []ResultEntry

func (r ResultEntries) MarshalJSON() ([]byte, error) {
	arr := make([]json.RawMessage, 0, len(r))
	for _, e := range r {
		b, err := json.Marshal([]interface{}{e.Key, e.Logs})
		if err != nil {
			return nil, err
		}
		arr = append(arr, b)
	}
	return json.Marshal(arr)
}

func (r *ResultEntries) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	out := make(ResultEntries, 0, len(raw))
	for _, item := range raw {
		var pair []json.RawMessage
		if err := json.Unmarshal(item, &pair); err != nil {
			continue
		}
		if len(pair) < 2 {
			continue
		}
		var key string
		_ = json.Unmarshal(pair[0], &key)
		var logs []Log
		_ = json.Unmarshal(pair[1], &logs)
		out = append(out, ResultEntry{Key: key, Logs: logs})
	}
	*r = out
	return nil
}

type DataSetJSON struct {
	UID            string        `json:"uid"`
	Time           int64         `json:"time"`
	Lang           string        `json:"lang"`
	RegionTimeZone int           `json:"region_time_zone"`
	Deleted        bool          `json:"deleted"`
	TypeMap        [][2]string   `json:"typeMap"`
	Result         ResultEntries `json:"result"`
}

var gachaOrder = []string{"2", "3", "1", "5", "102", "103"}

func orderKeys(m map[string][]Log) []string {
	seen := map[string]bool{}
	keys := []string{}
	for _, k := range gachaOrder {
		if _, ok := m[k]; ok {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	rest := []string{}
	for k := range m {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(keys, rest...)
}

func (d *DataSet) toJSON() DataSetJSON {
	entries := ResultEntries{}
	for _, k := range orderKeys(d.Result) {
		entries = append(entries, ResultEntry{Key: k, Logs: d.Result[k]})
	}
	tm := d.TypeMap
	if tm == nil {
		tm = [][2]string{}
	}
	return DataSetJSON{
		UID: d.UID, Time: d.Time, Lang: d.Lang, RegionTimeZone: d.RegionTZ,
		Deleted: d.Deleted, TypeMap: tm, Result: entries,
	}
}

func fromJSON(j DataSetJSON) *DataSet {
	res := map[string][]Log{}
	for _, e := range j.Result {
		res[e.Key] = e.Logs
	}
	tm := j.TypeMap
	if tm == nil {
		tm = [][2]string{}
	}
	return &DataSet{UID: j.UID, Time: j.Time, Lang: j.Lang, RegionTZ: j.RegionTimeZone, Deleted: j.Deleted, TypeMap: tm, Result: res}
}

func anyToString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	default:
		return ""
	}
}
