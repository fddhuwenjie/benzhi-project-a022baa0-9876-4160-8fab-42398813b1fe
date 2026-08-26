package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"iceguard/internal/audit"
	"iceguard/internal/domain"
	"iceguard/internal/httpapi"
	"iceguard/internal/store"
	"iceguard/internal/workflow"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "", "监听地址")
	self := flag.Bool("self-check", false, "执行闭环自检")
	flag.Parse()
	if *addr == "" {
		if p := os.Getenv("PORT"); p != "" {
			*addr = "127.0.0.1:" + p
		} else {
			*addr = "127.0.0.1:19081"
		}
	}
	st, err := store.New("data/iceguard.json")
	if err != nil {
		panic(err)
	}
	if err = st.CheckIntegrity(); err != nil {
		panic(err)
	}
	au := audit.New("data/audit.jsonl")
	_ = au.Verify()
	wf := workflow.New(st, au)
	srv := &http.Server{Addr: *addr, Handler: httpapi.New(wf).Handler()}
	if *self {
		go srv.ListenAndServe()
		time.Sleep(80 * time.Millisecond)
		if err := runSelfCheck(*addr); err != nil {
			panic(err)
		}
		srv.Close()
		return
	}
	fmt.Println("冰芯解冻服务监听 " + *addr)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}
func runSelfCheck(addr string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	base := "http://" + addr
	prefix := fmt.Sprintf("chk-%d-", time.Now().UnixNano())
	var b domain.IceCoreBatch
	if err := call(client, "POST", base+"/v1/batches", fmt.Sprintf(`{"core_code":"%s","drill_site":"%s","depth_interval_m":"10-20","initial_temperature_c":-30}`, prefix, prefix), prefix+"1", 0, &b); err != nil {
		return err
	}
	if err := call(client, "POST", base+"/v1/batches/"+b.ID+"/transport", `{"summary":"ok","min_temperature_c":-30,"max_temperature_c":-20,"allowed_min_c":-40,"allowed_max_c":0}`, prefix+"2", b.Revision, &b); err != nil {
		return err
	}
	if err := call(client, "POST", base+"/v1/batches/"+b.ID+"/thaw-plan", `{"author_id":"tech","stages":[{"index":1,"target_temperature_c":-1,"hold_minutes":5}]}`, prefix+"3", b.Revision, &b); err != nil {
		return err
	}
	if err := call(client, "POST", base+"/v1/batches/"+b.ID+"/thaw-plan/approve", `{"approver_id":"qa","plan_version":1,"approve":true}`, prefix+"4", b.Revision, &b); err != nil {
		return err
	}
	if err := call(client, "POST", base+"/v1/batches/"+b.ID+"/observations", `{"stage_index":1,"observed_temperature_c":-1,"meltwater_volume_ml":2,"hold_minutes":5,"appearance_note":"clear"}`, prefix+"5", b.Revision, &b); err != nil {
		return err
	}
	return call(client, "POST", base+"/v1/batches/"+b.ID+"/release-review", `{"reviewer_id":"independent","decision":"pass"}`, prefix+"6", b.Revision, &b)
}
func call(c *http.Client, method, url, body, rid string, rev int, out *domain.IceCoreBatch) error {
	req, _ := http.NewRequest(method, url, bytes.NewBufferString(body))
	req.Header.Set("X-Request-ID", rid)
	req.Header.Set("X-Expected-Revision", fmt.Sprint(rev))
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("request status %d: %s", resp.StatusCode, string(data))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}
	return nil
}
