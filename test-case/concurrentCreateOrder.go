package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"
)

func fetchPoolStats(n int, tag string) {
	resp, err := http.Get("http://localhost:8080/debug/pool")
	if err != nil {
		fmt.Printf("[%d] pool(%s) error: %v\n", n, tag, err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[%d] pool(%s) %s\n", n, tag, body)
}

func main() {
	var wg sync.WaitGroup

	start := make(chan struct{})

	for i := 0; i < 20; i++ {
		wg.Add(1)

		go func (n int)  {
			defer wg.Done()
			//body := []byte(`{"items":[{"product_id":2,"quantity":1}]}`)
			req, _ := http.NewRequest("GET", "http://localhost:8080/debug/slow", nil)

			<-start


			//start := time.Now()
            resp, err := http.DefaultClient.Do(req)
            if err != nil {
                fmt.Printf("[%d] error: %v\n", n, err)
                return
            }
            defer resp.Body.Close()
            // body, _ := io.ReadAll(resp.Body)
			// fmt.Printf("[%d] %v %s\n", n, time.Since(start), body)
		}(i)
	}

	close(start)//這時候關掉channel，所有被卡住的channel都會瞬間釋放
	fetchPoolStats(-1, "mid")    
	wg.Wait()
	fetchPoolStats(-1, "final")    
}
