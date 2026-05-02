import { check, randomSeed } from "k6";
import http from 'k6/http';
import { Rate, Trend } from 'k6/metrics';

const seed = parseInt(__ENV.SEED || '42', 10);
randomSeed(seed + __VU)

const ingestURL = __ENV.INGEST_URL || 'http://localhost:8090/ingest';

const injectionRate = parseInt(__ENV.INJECTION_RATE || '50', 10);
const injectionDuration = __ENV.SCENARIO_DURATION || '120s';

const systemIPs = (__ENV.SYSTEM_IPS || '').split(',').map(s => s.trim()).filter(Boolean);
const compromisedIPs = (__ENV.COMPROMISED_IPS || '').split(',').map(s => s.trim()).filter(Boolean);
const reachableTargets = {};

const ingestLatency = new Trend('ingest_req_latency', true);
const ingestSuccessRate = new Rate('ingest_success_rate');

function vlan(ip) {
    return ip.split('.').slice(0, 3).join('.');
}

for (const src of compromisedIPs) {
    reachableTargets[src] = systemIPs.filter(dst => vlan(dst) === vlan(src) && dst !== src);
}

export const options = {
    scenarios: {
        rate_injection: {
            executor: 'constant-arrival-rate',
            rate: injectionRate,
            timeUnit: '1s',
            duration: injectionDuration,
            preAllocatedVUs: 10,
            maxVUs: 50,
        },
    },
    thresholds: {
        dropped_iterations: ['count<1'],
    },
    tags: {
        test_type: 'matrix_alpha',
    },
    ext: {
        influxdb: {
            url: __ENV.INFLUX_URL,
            organization: __ENV.INFLUX_ORG,
            bucket: __ENV.INFLUX_BUCKET,
            token: __ENV.INFLUX_TOKEN,
        },
    },
};


function newDummyAlert(srcIP, dstIP) {
    const isExploit = Math.random() > 0.5;

    return {
        timestamp: new Date().toISOString(),
        flow_id: Math.floor(Math.random() * 10_000_000),
        in_iface: 'eth0',
        event_type: 'alert',
        src_ip: srcIP,
        src_port: Math.floor(Math.random() * 60_000) + 1024,
        dest_ip: dstIP,
        dest_port: isExploit ? 443 : 80,
        proto: 'TCP',
        ip_v: 4,
        alert: {
            action: 'allowed',
            gid: 1,
            signature_id: isExploit ? 2010935 : 2010936,
            rev: 1,
            signature: isExploit
                ? 'ET EXPLOIT Possible CVE-2023-XXXX Lateral Movement'
                : 'ET SCAN Nmap OS Detection Probe',
            category: isExploit
                ? 'Attempted Administrator Privilege Gain'
                : 'Information Leak',
            severity: isExploit ? 1 : 3,
        },
    };
}

export default function () {
    const srcIP = compromisedIPs[Math.floor(Math.random() * compromisedIPs.length)];

    const targets = reachableTargets[srcIP];
    if (!targets || targets.length === 0) {
        return;
    }

    const dstIP = targets[Math.floor(Math.random() * targets.length)];

    const payload = newDummyAlert(srcIP, dstIP);

    const start = Date.now();

    const res = http.post(
        ingestURL,
        JSON.stringify(payload),
        { headers: { 'Content-Type': 'application/json' } },
    );

    ingestLatency.add(Date.now() - start);

    ingestSuccessRate.add(
        check(res, { 'injected': r => r.status === 204 })
    );
}