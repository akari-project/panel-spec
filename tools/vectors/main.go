// SPDX-License-Identifier: Apache-2.0

// Command vectors 按 proto/node/v1 注释中的字节级定义，生成握手、会话加密、DNS 凭据加密、
// 快照校验和与 Agent 升级签名的测试向量。
// 模拟 Agent 与真实 Agent 都必须通过这些向量（spec/20 20.6）。
//
// 用法：go run ./tools/vectors > testdata/node-v1-vectors.json
package main

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
	"google.golang.org/protobuf/proto"

	nodev1 "github.com/akari-project/panel-spec/gen/go/node/v1"
)

const (
	dirAgentToPanel byte = 0x01
	dirPanelToAgent byte = 0x02
)

// field 按“字符串与字节串前置 4 字节大端长度”编码。
func field(b []byte) []byte {
	out := make([]byte, 4, 4+len(b))
	binary.BigEndian.PutUint32(out, uint32(len(b)))
	return append(out, b...)
}

func u32(v uint32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, v); return b }
func u64(v uint64) []byte { b := make([]byte, 8); binary.BigEndian.PutUint64(b, v); return b }

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func sum(b []byte) []byte { s := sha256.Sum256(b); return s[:] }

func mac(key, msg []byte) []byte { m := hmac.New(sha256.New, key); m.Write(msg); return m.Sum(nil) }

func fill(n int, start byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = start + byte(i)
	}
	return b
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func seal(key, nonce []byte, dir byte, seq uint64, plaintext []byte) []byte {
	aead, err := chacha20poly1305.NewX(key)
	must(err)
	aad := concat([]byte{dir}, u64(seq))
	return concat(nonce, aead.Seal(nil, nonce, plaintext, aad))
}

func main() {
	psk := fill(32, 0x01)
	nodeIDText := "0192f000-0000-7000-8000-000000000001"
	nodeIDBin, _ := hex.DecodeString("0192f000000070008000000000000001")
	tsMs := int64(1790000000000)
	helloNonce := fill(16, 0x40)
	protoVersion := uint32(1)
	configVersion := uint64(42)
	lastReportSeq := uint64(1000)

	agentPriv := fill(32, 0x10)
	panelPriv := fill(32, 0x20)
	agentPub, err := curve25519.X25519(agentPriv, curve25519.Basepoint)
	must(err)
	panelPub, err := curve25519.X25519(panelPriv, curve25519.Basepoint)
	must(err)

	caps := &nodev1.Capabilities{
		AgentVersion:  "0.1.0",
		Kernel:        nodev1.KernelType_KERNEL_TYPE_SINGBOX,
		KernelVersion: "1.12.0",
		DeployMode:    nodev1.DeployMode_DEPLOY_MODE_NODE,
		Kernels: []*nodev1.KernelSupport{{
			Kernel:           nodev1.KernelType_KERNEL_TYPE_SINGBOX,
			Version:          "1.12.0",
			StableProtocols:  []nodev1.Protocol{nodev1.Protocol_PROTOCOL_VLESS, nodev1.Protocol_PROTOCOL_TROJAN},
			StableTransports: []nodev1.Transport{nodev1.Transport_TRANSPORT_TCP, nodev1.Transport_TRANSPORT_WS},
		}},
		SupportsQuotaLease: true,
	}
	capsRaw, err := proto.MarshalOptions{Deterministic: true}.Marshal(caps)
	must(err)
	serverCapsRaw, err := proto.MarshalOptions{Deterministic: true}.Marshal(&nodev1.ControlPlaneCapabilities{
		PanelVersion:         "0.1.0",
		SupportsLeaseRelease: true,
	})
	must(err)

	helloMsg := concat(
		field([]byte("akari-node-hello-v1")), field([]byte(nodeIDText)), u64(uint64(tsMs)), field(helloNonce),
		u32(protoVersion), field(agentPub), field(sum(capsRaw)), u64(configVersion),
	)
	helloMAC := mac(psk, helloMsg)

	syncMode := uint32(nodev1.SyncMode_SYNC_MODE_FULL)
	ackMsg := concat(
		field([]byte("akari-node-hello-ack-v1")), field([]byte(nodeIDText)), field(agentPub), field(panelPub),
		u32(protoVersion), u32(syncMode), field(sum(serverCapsRaw)), u64(lastReportSeq),
	)
	ackMAC := mac(psk, ackMsg)

	shared, err := curve25519.X25519(agentPriv, panelPub)
	must(err)
	shared2, err := curve25519.X25519(panelPriv, agentPub)
	must(err)
	if !hmac.Equal(shared, shared2) {
		panic("X25519 mismatch")
	}
	info := concat([]byte("akari-node-session-v1"), nodeIDBin, agentPub, panelPub)
	keys := make([]byte, 64)
	_, err = io.ReadFull(hkdf.New(sha256.New, shared, helloNonce, info), keys)
	must(err)
	kUp, kDown := keys[:32], keys[32:]

	upEnv, err := proto.Marshal(&nodev1.Envelope{
		Ack: 0, TsMs: tsMs + 1000, IdemKey: "0192f000-0000-7000-8000-0000000000aa",
		Body: &nodev1.Envelope_ReportStatus{ReportStatus: &nodev1.ReportStatus{ConfigVersion: configVersion}},
	})
	must(err)
	upNonce := fill(24, 0x60)
	upSealed := seal(kUp, upNonce, dirAgentToPanel, 1, upEnv)

	downEnv, err := proto.Marshal(&nodev1.Envelope{
		Ack: 1, TsMs: tsMs + 2000, IdemKey: "0192f000-0000-7000-8000-0000000000bb",
		Body: &nodev1.Envelope_AckOnly{AckOnly: &nodev1.Ack{}},
	})
	must(err)
	downNonce := fill(24, 0x80)
	downSealed := seal(kDown, downNonce, dirPanelToAgent, 1, downEnv)

	kDNS := make([]byte, 32)
	_, err = io.ReadFull(hkdf.New(sha256.New, psk, nil, []byte("akari-dns-secret-v1")), kDNS)
	must(err)
	dnsPlain := []byte(`{"provider":"cloudflare","api_token":"example"}`)
	dnsNonce := fill(24, 0xa0)
	aead, err := chacha20poly1305.NewX(kDNS)
	must(err)
	dnsSealed := concat(dnsNonce, aead.Seal(nil, dnsNonce, dnsPlain, nil))

	snapshotRaw, err := proto.Marshal(&nodev1.Snapshot{
		ConfigVersion: configVersion,
		Kernel:        nodev1.KernelType_KERNEL_TYPE_SINGBOX,
		OfflinePolicy: &nodev1.OfflinePolicy{OfflineLeaseBytes: 64 << 20, OfflineMaxSeconds: 86400},
	})
	must(err)

	// AgentUpgrade 签名（messages.proto，spec/40 DEP-09）：
	// "akari-agent-upgrade-v1" | version | sha256，sha256 为 32 字节字节串，同样加长度前缀。
	upgradeSeed := fill(ed25519.SeedSize, 0xc0)
	upgradeKey := ed25519.NewKeyFromSeed(upgradeSeed)
	upgradeVersion := "0.1.1"
	upgradeDigest := sum([]byte("akari node-agent 0.1.1 test artifact"))
	upgradeInput := concat(field([]byte("akari-agent-upgrade-v1")), field([]byte(upgradeVersion)), field(upgradeDigest))
	upgradeSig := ed25519.Sign(upgradeKey, upgradeInput)
	if !ed25519.Verify(upgradeKey.Public().(ed25519.PublicKey), upgradeInput, upgradeSig) {
		panic("ed25519 verify failed")
	}

	h := hex.EncodeToString
	out := map[string]any{
		"description": "node.v1 握手、会话加密、DNS 凭据加密、快照校验和与 Agent 升级签名的测试向量，定义见 proto/node/v1/envelope.proto 与 messages.proto 的注释。所有字节串为十六进制；agent_upgrade 的私钥种子只用于测试。",
		"inputs": map[string]any{
			"psk":                     h(psk),
			"node_id":                 nodeIDText,
			"ts_ms":                   tsMs,
			"hello_nonce":             h(helloNonce),
			"proto_version":           protoVersion,
			"config_version":          configVersion,
			"last_report_seq":         lastReportSeq,
			"sync_mode":               syncMode,
			"agent_ephemeral_private": h(agentPriv),
			"panel_ephemeral_private": h(panelPriv),
			"capabilities_raw":        h(capsRaw),
			"server_capabilities_raw": h(serverCapsRaw),
		},
		"hello": map[string]any{
			"agent_ephemeral_pubkey": h(agentPub),
			"mac_input":              h(helloMsg),
			"mac":                    h(helloMAC),
		},
		"hello_ack": map[string]any{
			"panel_ephemeral_pubkey": h(panelPub),
			"mac_input":              h(ackMsg),
			"mac":                    h(ackMAC),
		},
		"session": map[string]any{
			"shared_secret":      h(shared),
			"hkdf_info":          h(info),
			"key_agent_to_panel": h(kUp),
			"key_panel_to_agent": h(kDown),
			"up_seq":             1,
			"up_nonce":           h(upNonce),
			"up_envelope":        h(upEnv),
			"up_sealed":          h(upSealed),
			"down_seq":           1,
			"down_nonce":         h(downNonce),
			"down_envelope":      h(downEnv),
			"down_sealed":        h(downSealed),
		},
		"dns_secret": map[string]any{
			"key":       h(kDNS),
			"plaintext": string(dnsPlain),
			"nonce":     h(dnsNonce),
			"sealed":    h(dnsSealed),
		},
		"agent_upgrade": map[string]any{
			"key_id":          1,
			"private_seed":    h(upgradeSeed),
			"public_key":      h(upgradeKey.Public().(ed25519.PublicKey)),
			"version":         upgradeVersion,
			"sha256":          h(upgradeDigest),
			"signature_input": h(upgradeInput),
			"signature":       h(upgradeSig),
		},
		"sync_full": map[string]any{
			"snapshot": h(snapshotRaw),
			"checksum": h(sum(snapshotRaw)),
		},
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	must(enc.Encode(out))
}
