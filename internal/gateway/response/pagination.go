package response

import (
	"fmt"
	"strconv"
	"strings"
)

func ParsePathID(raw string) (int64, error) {
	value := strings.TrimSpace(raw)
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func ParseOptionalQueryID(raw string) (*int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	id, err := ParsePathID(value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func ParsePage(raw string) (int32, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 1, nil
	}
	page, err := strconv.Atoi(value)
	if err != nil || page <= 0 {
		return 0, fmt.Errorf("invalid page")
	}
	return int32(page), nil
}

func ParsePageSize(raw string) (int32, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 10, nil
	}
	pageSize, err := strconv.Atoi(value)
	if err != nil || pageSize <= 0 {
		return 0, fmt.Errorf("invalid page_size")
	}
	if pageSize > 100 {
		return 100, nil
	}
	return int32(pageSize), nil
}
