import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';
import { randomIntBetween, uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

/**
 * Usage:
 * k6 run -e TARGET_IP=34.14.198.214 -e RATE=10 loadtest.js
 */

const TARGET_IP = __ENV.TARGET_IP;
if (!TARGET_IP) {
  throw new Error('TARGET_IP env variable is required (e.g. -e TARGET_IP=34.14.198.214)');
}

const RATE = Number(__ENV.RATE);
if (!RATE) {
    throw new Error('RATE env variable is required (e.g. -e RATE=10)');
}

const URL = `http://${TARGET_IP}:8080/job`;

// Define status code counters
const status200 = new Counter('http_status_200');
const status429 = new Counter('http_status_429');
const status4xx = new Counter('http_status_4xx');
const status5xx = new Counter('http_status_5xx');

export const options = {
    scenarios: {
      load_test: {
        executor: 'ramping-arrival-rate',
        timeUnit: '1s',
  
        startRate: 1,          // start very slow
        preAllocatedVUs: 100,
        maxVUs: 300,
  
        stages: [
            { target: Math.max(1, Math.round(RATE * 0.25)), duration: '15s' },
            { target: Math.round(RATE * 0.5), duration: '15s' },
            { target: Math.round(RATE * 0.75),  duration: '15s' },
            { target: RATE,                    duration: '300s' },
            { target: Math.round(RATE * 0.5),  duration: '15s' },
            { target: 0,                       duration: '20s' },
          ],
      },
    },
  
    summaryTrendStats: ["avg", "min", "med", "max", "p(50)", "p(90)", "p(95)", "p(99)"],
    
    thresholds: {
      'http_status_200': [],
      'http_status_429': [],
      'http_status_4xx': [],
      'http_status_5xx': [],
    },
  };

// 30 C++ templates
const cppTemplates = [
`#include <bits/stdc++.h>
int main() { int a=1,b=2; std::cout<<a+b<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { for(int i=0;i<5;i++) std::cout<<i<<" "; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::vector<int> v={1,2,3}; for(int x:v) std::cout<<x<<" "; return 0; }`,
`#include <bits/stdc++.h>
int main() { int arr[]={3,1,2}; std::sort(arr,arr+3); for(int i:arr) std::cout<<i<<" "; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::string s="hello"; std::cout<<s<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::cout<<sqrt(16)<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::map<int,int> m; m[1]=10; std::cout<<m[1]<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::set<int> s; s.insert(5); s.insert(1); std::cout<<*s.begin()<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::queue<int> q; q.push(1); std::cout<<q.front()<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::stack<int> st; st.push(2); std::cout<<st.top()<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { int x=10; int y=20; std::cout<<x*y<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { int sum=0; for(int i=1;i<=10;i++) sum+=i; std::cout<<sum<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::vector<int> v={1,2,3}; std::reverse(v.begin(),v.end()); for(int x:v) std::cout<<x<<" "; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::string s="abc"; std::cout<<s+s<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::cout<<pow(2,3)<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { int n=5; while(n>0){ std::cout<<n<<" "; n--; } return 0; }`,
`#include <bits/stdc++.h>
int main() { int a[3]={1,2,3}; for(int i: a) std::cout<<i*i<<" "; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::vector<int> v={5,3,1}; std::sort(v.begin(),v.end()); for(int x:v) std::cout<<x<<" "; return 0; }`,
`#include <bits/stdc++.h>
int main() { char c='A'; std::cout<<(char)(c+1)<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { bool b=true; std::cout<<b<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { for(int i=0;i<3;i++) std::cout<<"Hi "; return 0; }`,
`#include <bits/stdc++.h>
int main() { int a=10; std::cout<<a%3<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::cout<<'A'+'B'<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::vector<int> v; v.push_back(1); std::cout<<v[0]<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { int x=7; std::cout<<(x>5)<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { int a=3,b=4; std::cout<<a+b<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::string s="load"; s+="test"; std::cout<<s<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::cout<<abs(-42)<<std::endl; return 0; }`,
`#include <bits/stdc++.h>
int main() { int arr[]={1,2,3}; for(int x:arr) std::cout<<x+1<<" "; return 0; }`,
`#include <bits/stdc++.h>
int main() { std::cout<<"Random test"<<std::endl; return 0; }`,
];

function padCppToSize(code, targetBytes) {
    const currentSize = code.length;
    if (currentSize >= targetBytes) {
      return code;
    }
  
    const paddingNeeded = targetBytes - currentSize - 10;
    const filler = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789\n';
    const repeats = Math.ceil(paddingNeeded / filler.length);
  
    const padding = filler.repeat(repeats).slice(0, paddingNeeded);
  
    return `${code}\n\n/*\n${padding}\n*/\n`;
}

export default function () {
    const template = cppTemplates[randomIntBetween(0, cppTemplates.length - 1)];
    const unique = randomIntBetween(0, 1_000_000);
  
    let code = template.replace(
      'return 0;',
      `printf("unique${unique}\\n");\nreturn 0;`
    );
  
    // Random size between 20 KB and 1 MB
    const targetSizeBytes = randomIntBetween(
      20 * 1024,
      1024 * 1024
    );
  
    code = padCppToSize(code, targetSizeBytes);
  
    const metadata = JSON.stringify({
      executionEngine: 'c++',
      tags: ['LoadTest'],
    });
  
    const payload = {
      metadata: metadata,
      code: http.file(code, 'code.cpp', 'text/plain'),
    };

    const idempotencyKey = uuidv4();
  
    const res = http.post(URL, payload, {timeout: '3s', headers: {
      'Idempotency-Key': idempotencyKey,
    },});
    if (res.status === 200) {
      status200.add(1);
    } else if (res.status === 429) {
      status429.add(1);
    } else if (res.status >= 400 && res.status < 500) {
      status4xx.add(1);
    } else if (res.status >= 500) {
      status5xx.add(1);
    }
  
    check(res, {
      'status is 200': (r) => r.status === 200,
    });
}