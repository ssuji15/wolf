package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

func main() {
	url := "http://localhost:8080/job"

	payload := map[string]interface{}{
		"code":            "#include <bits/stdc++.h>\nusing namespace std;\nstruct TreeNode { int data; TreeNode *left; TreeNode *right; TreeNode(int val) : data(val), left(nullptr), right(nullptr) {} };\nclass Solution { public: vector<vector<int>> zigzagLevelOrder(TreeNode* root) { vector<vector<int>> result; if (root == nullptr) { return result; } queue<TreeNode*> nodesQueue; nodesQueue.push(root); bool leftToRight = true; while (!nodesQueue.empty()) { int size = nodesQueue.size(); vector<int> row(size); for (int i = 0; i < size; i++) { TreeNode* node = nodesQueue.front(); nodesQueue.pop(); int index = leftToRight ? i : (size - 1 - i); row[index] = node->data; if (node->left) nodesQueue.push(node->left); if (node->right) nodesQueue.push(node->right); } leftToRight = !leftToRight; result.push_back(row); } return result; } };\nvoid printResult(const vector<vector<int>>& result) { for (const auto& row : result) { for (int val : row) cout << val << \" \"; cout << endl; } }\nint main() { TreeNode* root = new TreeNode(1); root->left = new TreeNode(2); root->right = new TreeNode(3); root->left->left = new TreeNode(4); root->left->right = new TreeNode(5); root->right->left = new TreeNode(6); root->right->right = new TreeNode(7); Solution solution; auto result = solution.zigzagLevelOrder(root); printResult(result); return 0; }",
		"executionEngine": "c++",
		"tags":            []string{"Tree"},
	}

	jsonData, _ := json.Marshal(payload)

	totalRequests := 100
	ratePerSecond := 5

	ticker := time.NewTicker(time.Second / time.Duration(ratePerSecond))
	defer ticker.Stop()

	var wg sync.WaitGroup
	client := &http.Client{}

	for i := 1; i <= totalRequests; i++ {
		<-ticker.C // enforce rate limit

		wg.Add(1)

		go func(n int) {
			defer wg.Done()

			req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
			if err != nil {
				fmt.Printf("Request %d: error creating request: %v\n", n, err)
				return
			}

			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				fmt.Printf("Request %d: error sending request: %v\n", n, err)
				return
			}
			defer resp.Body.Close()

			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Fatal(err)
			}

			fmt.Printf("Request %d -> Status: %d, content: %s\n", n, resp.StatusCode, string(bodyBytes))
		}(i)
	}

	wg.Wait()
	fmt.Println("All requests completed")
}
