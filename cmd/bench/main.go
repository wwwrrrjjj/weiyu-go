// Command bench 是零依赖的 HTTP 压测小工具。
//
//	go run ./cmd/bench -url http://127.0.0.1:8080/api/v1/posts/1 -n 5000 -c 100 \
//	  -mysql-dsn 'user:pass@tcp(127.0.0.1:3306)/db?parseTime=True' -tag cache
//
// 传了 -mysql-dsn 时，会在压测前后读取 MySQL 的 Com_select 增量，
// 用来验证「缓存命中后到底还有没有回源」，这比只看 QPS 更能说明问题。
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type headerFlags []string

func (h *headerFlags) String() string { return strings.Join(*h, ", ") }
func (h *headerFlags) Set(v string) error {
	*h = append(*h, v)
	return nil
}

func main() {
	url := flag.String("url", "", "目标地址（必填）")
	n := flag.Int("n", 5000, "统计的请求数")
	c := flag.Int("c", 100, "并发数")
	warmup := flag.Int("warmup", 200, "预热请求数（不计入统计，用于填缓存）")
	timeout := flag.Duration("timeout", 5*time.Second, "单请求超时")
	tag := flag.String("tag", "", "结果标签，如 cache / nocache")
	mysqlDSN := flag.String("mysql-dsn", "", "可选：用于统计 Com_select 增量")
	var headers headerFlags
	flag.Var(&headers, "H", "附加请求头，如 'Authorization: Bearer xxx'，可重复")
	flag.Parse()

	if *url == "" {
		fmt.Fprintln(os.Stderr, "必须指定 -url")
		flag.Usage()
		os.Exit(2)
	}
	if *c <= 0 || *n <= 0 {
		fmt.Fprintln(os.Stderr, "-n 与 -c 必须大于 0")
		os.Exit(2)
	}

	client := &http.Client{
		Timeout: *timeout,
		Transport: &http.Transport{
			MaxIdleConns:        *c * 2,
			MaxIdleConnsPerHost: *c,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	// 预热：把缓存填上，避免把首批回源算进统计
	for i := 0; i < *warmup; i++ {
		_, _ = doRequest(client, *url, headers)
	}

	var before int64
	if *mysqlDSN != "" {
		var err error
		before, err = comSelect(*mysqlDSN)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 Com_select 失败:", err)
			os.Exit(1)
		}
	}

	start := time.Now()
	latencies, failed := load(client, *url, headers, *n, *c)
	elapsed := time.Since(start)

	var after int64
	if *mysqlDSN != "" {
		var err error
		after, err = comSelect(*mysqlDSN)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 Com_select 失败:", err)
			os.Exit(1)
		}
	}

	printReport(*tag, *url, *n, *c, elapsed, latencies, failed)
	if *mysqlDSN != "" {
		delta := after - before
		fmt.Printf("DB SELECT 增量 : %d（%.3f 条/请求）\n", delta, float64(delta)/float64(*n))
	}
}

func load(client *http.Client, url string, headers []string, n, concurrency int) ([]time.Duration, int64) {
	jobs := make(chan struct{}, concurrency)
	results := make(chan time.Duration, n)
	var failed atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				latency, err := doRequest(client, url, headers)
				if err != nil {
					failed.Add(1)
					continue
				}
				results <- latency
			}
		}()
	}

	go func() {
		for i := 0; i < n; i++ {
			jobs <- struct{}{}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	latencies := make([]time.Duration, 0, n)
	for d := range results {
		latencies = append(latencies, d)
	}
	return latencies, failed.Load()
}

func doRequest(client *http.Client, url string, headers []string) (time.Duration, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	for _, h := range headers {
		if k, v, ok := strings.Cut(h, ":"); ok {
			req.Header.Set(strings.TrimSpace(k), strings.TrimSpace(v))
		}
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	latency := time.Since(start)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return latency, fmt.Errorf("status %d", resp.StatusCode)
	}
	return latency, nil
}

func printReport(tag, url string, n, concurrency int, elapsed time.Duration, latencies []time.Duration, failed int64) {
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	label := ""
	if tag != "" {
		label = "[" + tag + "] "
	}
	fmt.Printf("==== bench %s====\n", label)
	fmt.Printf("目标          : GET %s\n", url)
	fmt.Printf("并发/请求     : %d / %d\n", concurrency, n)
	fmt.Printf("耗时          : %.2fs\n", elapsed.Seconds())
	fmt.Printf("QPS           : %.1f\n", float64(len(latencies))/elapsed.Seconds())
	fmt.Printf("成功/失败     : %d / %d\n", len(latencies), failed)
	if len(latencies) > 0 {
		fmt.Printf("延迟ms        : p50=%.2f p90=%.2f p95=%.2f p99=%.2f max=%.2f\n",
			percentile(latencies, 0.50), percentile(latencies, 0.90), percentile(latencies, 0.95),
			percentile(latencies, 0.99), float64(latencies[len(latencies)-1].Microseconds())/1000)
	}
}

func percentile(sorted []time.Duration, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted))*p) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return float64(sorted[idx].Microseconds()) / 1000
}

func comSelect(dsn string) (int64, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	var name string
	var value int64
	if err := db.QueryRow("SHOW GLOBAL STATUS LIKE 'Com_select'").Scan(&name, &value); err != nil {
		return 0, err
	}
	return value, nil
}
