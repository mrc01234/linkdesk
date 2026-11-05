package handler

import (
	"encoding/json"
	"fmt"
	"linkdesk/model"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func parseListOptions(r *http.Request) (model.ListOptions, error) {
	q := r.URL.Query()
	opt := model.ListOptions{Query: strings.TrimSpace(q.Get("q")), Status: q.Get("status"), Page: 1, PageSize: 10}
	if opt.Status == "" {
		opt.Status = "all"
	}
	for _, field := range []struct {
		name string
		dest *int
		max  int
	}{{"page", &opt.Page, 1000000}, {"page_size", &opt.PageSize, 100}} {
		if values, present := q[field.name]; present {
			if len(values) != 1 {
				return opt, fmt.Errorf("%s must occur once", field.name)
			}
			n, err := strconv.Atoi(values[0])
			if err != nil || n < 1 || n > field.max {
				return opt, fmt.Errorf("%s must be between 1 and %d", field.name, field.max)
			}
			*field.dest = n
		}
	}
	if len(opt.Query) > 200 {
		return opt, fmt.Errorf("q must not exceed 200 bytes")
	}
	if opt.Status != "all" && opt.Status != "active" && opt.Status != "expired" {
		return opt, fmt.Errorf("status must be all, active or expired")
	}
	return opt, nil
}

func pageLink(opt model.ListOptions, page int) string {
	q := url.Values{"q": {opt.Query}, "status": {opt.Status}, "page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(opt.PageSize)}}
	return "/urls?" + q.Encode()
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON: %v", err)
	}
}
