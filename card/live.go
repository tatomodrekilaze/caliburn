package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Live broadcasts over the spine. They are cooperative, so the battery
// does not task them. A track on the Kutaisi final is only marked withheld.

type contact struct {
	call       string
	lat, lon   float64
	altM       float64
	trackDeg   float64
	onFinalBox bool
}

var (
	liveMu     sync.Mutex
	liveAc     []contact
	liveAt     time.Time
	liveErr    string
	liveClient = &http.Client{Timeout: 12 * time.Second}
)

func fetchLive() {
	ac, err := fromOpenSky()
	if len(ac) == 0 {
		ac2, err2 := fromADSB()
		if len(ac2) > 0 {
			ac = ac2
			err = nil
		} else if err != nil {
			err = err2
		}
	}
	markFinal(ac)
	liveMu.Lock()
	liveAc = ac
	liveAt = time.Now()
	if err != nil && len(ac) == 0 {
		liveErr = err.Error()
	} else {
		liveErr = ""
	}
	liveMu.Unlock()
}

func liveSnapshot() ([]contact, time.Time, string) {
	liveMu.Lock()
	defer liveMu.Unlock()
	out := append([]contact(nil), liveAc...)
	return out, liveAt, liveErr
}

func markFinal(ac []contact) {
	for i := range ac {
		x := (ac[i].lon - lon0) * mLon
		y := (ac[i].lat - lat0) * mLat
		ac[i].onFinalBox = onFinal(x, y, ac[i].altM)
	}
}

func fromOpenSky() ([]contact, error) {
	south, west, north, east := frameBox()
	url := fmt.Sprintf(
		"https://opensky-network.org/api/states/all?lamin=%f&lomin=%f&lamax=%f&lomax=%f",
		south, west, north, east,
	)
	body, err := getJSON(url)
	if err != nil {
		return nil, err
	}
	var doc struct {
		States [][]any `json:"states"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var out []contact
	for _, row := range doc.States {
		if len(row) < 11 {
			continue
		}
		if on, ok := row[8].(bool); ok && on {
			continue
		}
		lon, okLon := asFloat(row[5])
		lat, okLat := asFloat(row[6])
		if !okLon || !okLat {
			continue
		}
		alt, okAlt := asFloat(row[7])
		if !okAlt {
			continue
		}
		call, _ := row[1].(string)
		call = strings.TrimSpace(call)
		if call == "" {
			call, _ = row[0].(string)
		}
		track, _ := asFloat(row[10])
		out = append(out, contact{call: call, lat: lat, lon: lon, altM: alt, trackDeg: track})
	}
	return out, nil
}

func fromADSB() ([]contact, error) {
	south, west, north, east := frameBox()
	lat := (south + north) / 2
	lon := (west + east) / 2
	url := fmt.Sprintf("https://api.adsb.lol/v2/lat/%f/lon/%f/dist/140", lat, lon)
	body, err := getJSON(url)
	if err != nil {
		return nil, err
	}
	var doc struct {
		AC []struct {
			Flight string          `json:"flight"`
			Hex    string          `json:"hex"`
			Lat    float64         `json:"lat"`
			Lon    float64         `json:"lon"`
			Alt    json.RawMessage `json:"alt_baro"`
			Track  float64         `json:"track"`
			Seen   float64         `json:"seen"`
		} `json:"ac"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var out []contact
	for _, row := range doc.AC {
		if row.Lat < south || row.Lat > north || row.Lon < west || row.Lon > east {
			continue
		}
		if row.Lat == 0 && row.Lon == 0 {
			continue
		}
		if row.Seen > 45 {
			continue
		}
		altFt, ok := feet(row.Alt)
		if !ok {
			continue
		}
		call := strings.TrimSpace(row.Flight)
		if call == "" {
			call = strings.ToUpper(row.Hex)
		}
		out = append(out, contact{
			call: call, lat: row.Lat, lon: row.Lon,
			altM: altFt * 0.3048, trackDeg: row.Track,
		})
	}
	return out, nil
}

func feet(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil || s == "ground" {
			return 0, false
		}
		v, err := strconv.ParseFloat(s, 64)
		return v, err == nil
	}
	var v float64
	if json.Unmarshal(raw, &v) != nil {
		return 0, false
	}
	return v, true
}

func getJSON(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Caliburn/1.0 (local scope)")
	res, err := liveClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("air picture %s", res.Status)
	}
	return body, nil
}

func asFloat(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

func georgiaNow() time.Time {
	loc, err := time.LoadLocation("Asia/Tbilisi")
	if err != nil {
		loc = time.FixedZone("GET", 4*3600)
	}
	return time.Now().In(loc)
}

func levelText(altM float64) string {
	ft := altM / 0.3048
	if ft >= 10000 {
		return fmt.Sprintf("FL%d", int(ft/100+0.5))
	}
	return fmt.Sprintf("%d ft", int(ft+0.5))
}
