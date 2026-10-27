// Package remote подключается к чужому компьютеру по IP через административные
// общие ресурсы Windows (\\ip\C$) и находит на нём диски.
//
// Дальше с диском работает обычный поиск: путь вида \\192.168.150.7\D$ для
// него такая же папка, как и любая другая.
package remote

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Disk — диск удалённого компьютера, доступный по административной шаре.
type Disk struct {
	Letter string `json:"letter"` // "C"
	Path   string `json:"path"`   // \\ip\C$
	Total  int64  `json:"total"`  // байт; 0, если узнать не удалось
	Free   int64  `json:"free"`
}

// scanTimeout — сколько ждать ответа при проверке дисков: недоступный хост
// иначе подвешивает проверку на десятки секунд.
const scanTimeout = 6 * time.Second

// ParseHost проверяет адрес и возвращает его в каноничном виде.
// Принимается только IPv4: это исключает пути и служебные символы в адресе.
func ParseHost(s string) (string, error) {
	s = strings.TrimSpace(s)
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() == nil || strings.Contains(s, ":") {
		return "", errors.New("Введите IP-адрес в формате 192.168.150.7")
	}
	return ip.To4().String(), nil
}

// SharePath возвращает путь административной шары диска: \\ip\C$.
func SharePath(host, letter string) string {
	return `\\` + host + `\` + letter + `$`
}

// ScanDisks проверяет буквы A–Z на хосте и возвращает найденные диски по алфавиту.
// Подключение (Connect) к этому моменту уже должно быть установлено.
func ScanDisks(ctx context.Context, host string) ([]Disk, error) {
	ctx, cancel := context.WithTimeout(ctx, scanTimeout)
	defer cancel()

	var (
		mu    sync.Mutex
		found []Disk
		wg    sync.WaitGroup
	)
	for c := 'A'; c <= 'Z'; c++ {
		wg.Add(1)
		go func(letter string) {
			defer wg.Done()
			path := SharePath(host, letter)
			if !exists(ctx, path+`\`) {
				return
			}
			d := Disk{Letter: letter, Path: path}
			d.Total, d.Free = diskSpace(path + `\`)
			mu.Lock()
			found = append(found, d)
			mu.Unlock()
		}(string(c))
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}

	mu.Lock()
	defer mu.Unlock()
	if err := ctx.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	out := append([]Disk(nil), found...)
	sort.Slice(out, func(i, j int) bool { return out[i].Letter < out[j].Letter })
	if len(out) == 0 {
		return nil, fmt.Errorf("на %s не найдено доступных дисков: нужна учётная запись администратора и включённые общие ресурсы C$", host)
	}
	return out, nil
}

// exists проверяет путь, не дожидаясь зависшего сетевого вызова дольше ctx.
func exists(ctx context.Context, path string) bool {
	ok := make(chan bool, 1)
	go func() {
		_, err := os.Stat(path)
		ok <- err == nil
	}()
	select {
	case v := <-ok:
		return v
	case <-ctx.Done():
		return false
	}
}
