# REALITY 与 anyTLS 协议对比调研

> 调研日期:2026-07-06
> 调研对象:REALITY(Xray-core `transport/internet/reality`)、anyTLS(`anytls/anytls-go`)

## 一句话定位

- **REALITY**:一个**传输/握手安全层**(不是完整代理协议,需配合 VLESS/Trojan 等),解决的是「**主动探测**」和「**免域名/免证书**」问题。
- **anyTLS**:一个**完整的代理协议**(自带认证、多路复用、SOCKS5 入站),解决的是「**TLS-in-TLS(嵌套 TLS)指纹识别**」问题。

两者解决的是**不同层面的威胁**,技术上不冲突,甚至可以组合(`ssrlive/reality-rs` 的 "AnyReality" 就是 REALITY 包裹 anyTLS)。

---

## 一、REALITY 的核心机制

> 源码:`transport/internet/reality/reality.go`(外部核心在依赖 `github.com/xtls/reality`)

思路:「**借真实网站的壳,在 TLS 握手阶段就区分真假客户端**」。

1. **客户端用 uTLS 模拟真实浏览器指纹**发起 ClientHello,SNI 指向一个真实目标网站(dest,如 `microsoft.com`)。
2. **把认证信息藏进 ClientHello 的 `SessionId` 字段(32 字节)**:
   - 写入版本号、时间戳、ShortId;
   - 用 **ECDH**(客户端临时密钥 × 服务端配置的 PublicKey)算出共享密钥 `AuthKey`,经 HKDF 派生后用 **AES-GCM (AEAD)** 加密 SessionId 前 16 字节,nonce 取 `hello.Random` 的后 12 字节(`reality.go:163-175`)。
3. **服务端收到 ClientHello 后分流**:
   - 认证通过(真客户端)→ **「劫持」连接**:不再走标准 TLS,而是用 ECDH 共享密钥直接加密应用数据,伪装成 TLS ApplicationData。服务端**不需要、也不持有** dest 的真实证书。
   - 认证不通过(GFW 主动探测 / 无关流量)→ **透明转发给真实 dest**,探测者看到的是完全合法的真实 TLS 握手和真实网站证书/响应。
4. **客户端反向验证服务端**:服务端在证书里用 `ed25519 + HMAC-SHA512(AuthKey)` 签名,证明自己是 REALITY 服务端(真实 dest 不可能有此签名);还支持 **ML-DSA-65 后量子签名**增强(`reality.go:84-103`)。
5. 若客户端发现收到的是真实证书(疑似 MITM/重定向),会**模拟浏览器爬虫**访问 dest 真实页面,制造「我是真浏览器」的假象再断开(`reality.go:184-274`)。

> 关键点:REALITY 握手后**不是标准 TLS**,而是自建 AEAD 伪装成 TLS。被代理流量直接走这层,**不存在「TLS 套 TLS」的双层嵌套**。

---

## 二、anyTLS 的核心机制

> 源码:`anytls/anytls-go`(`docs/protocol.md`、`docs/faq.md`、`proxy/padding/padding.go`)

思路:「**标准 TLS 之上,用会话层 + 可更新的 padding 方案来打乱内层 TLS 的长度特征**」。

1. **标准 TLS 握手**(自己持有证书,可自签,客户端配 root CA 验证)。
2. **TLS 握手完成后,客户端立即发认证**:`sha256(password) | padding0 len | padding0`(固定 34 字节开销)。认证失败则关闭或 fallback 到 HTTP 服务(类 trojan)。
3. **会话层 framing**(TLS 之上多一层):`command | streamId | dataLen | data`,命令包括 SYN/PSH/FIN(流控)、`cmdWaste`(padding)、`cmdSettings`/`cmdServerSettings`(版本协商)、`cmdUpdatePaddingScheme`(服务端动态下发新 padding 方案)、心跳(v2)。
4. **内置多路复用**:`TCP Proxy → Stream → Session → TLS → TCP`,复用空闲会话(优先最新、清理最老)。
5. **对抗 TLS-in-TLS 的核心——PaddingScheme**(定义前 N 个包如何分片+填充):
   - 例:`2=400-500,c,500-1000,c,...` 表示「第 2 号包(通常是内层 TLS ClientHello)拆成 5 个 400-500/500-1000 字节的子包发送」;
   - `c` 是检查符:若用户数据在中途已发完,就不再补填充包;
   - 服务端可通过 `cmdUpdatePaddingScheme` **动态下发新方案**,客户端首连用默认方案,收到更新后后续连接换用。设计意图:即使默认特征被 GFW 列黑名单,也只有首连接的前几个包暴露特征(`docs/protocol.md:130`)。
   - 实现见 `proxy/padding/padding.go` 的 `GenerateRecordPayloadSizes`。

FAQ 还自陈了**已知弱点**(`docs/faq.md:76-89`):TLS-over-TLS 比真实 h2 需要更多握手往返、未处理下行流量、PaddingScheme 语法有限、多包几乎同时发送可能被时序-包长统计识别、TLS-over-TLS 开销导致包长超 MTU 等;并直言「这不是 HTTP 服务器,仍可能存在主动探测问题」。

---

## 三、核心区别对比

| 维度 | REALITY | anyTLS |
|---|---|---|
| **定位** | 传输/握手安全层,需配合 VLESS/Trojan 等 | 完整代理协议(自带认证、多路复用、SOCKS5) |
| **主要对抗威胁** | 主动探测 + 免域名/免证书 | TLS-in-TLS(嵌套 TLS)指纹识别 |
| **是否产生双层 TLS** | **否**(劫持握手后用自建 AEAD 伪装 TLS,无嵌套) | **是**(外层标准 TLS + 内层被代理的 TLS) |
| **域名/证书** | **不需要**,借真实 dest 的证书 | **需要**(自签 + 客户端 root CA,或真实域名+LE) |
| **TLS 握手** | 用真实 dest 的 SNI,但不完成标准 TLS,ECDH 自建加密 | 标准 TLS 握手(自己持证书) |
| **认证时机** | TLS 握手**阶段**(SessionId 内 AEAD 加密) | TLS 握手**之后**(发 sha256(password)) |
| **认证方式** | ECDH 共享密钥 + AEAD 加密的版本/时间/ShortId | 明文 sha256(password) + padding |
| **主动探测防御** | 透传给真实 dest,探测者看到合法真网站(强) | fallback 到 HTTP(自陈仍有探测弱点) |
| **流量整形** | 配合 XTLS Vision(padding/direct-copy,逻辑写死) | PaddingScheme(分包+填充,**可动态更新**) |
| **多路复用** | 无,靠上层 VLESS/mux | 内置 Session/Stream(smux 思路) |
| **下行流量处理** | Vision 双向处理 | 自陈未处理(弱点) |
| **客户端指纹** | uTLS 模拟真实浏览器 | 参考实现不处理 ClientHello 特征(FAQ:非重点) |

---

## 四、最本质的技术差异

**1. 威胁模型不同。**
- REALITY 防的是「**审查者主动连上你的服务器,验证它是不是代理**」。所以它的核心是:探测者连进来 → 看到真实网站;真客户端连进来 → 用 SessionId 里的 AEAD 暗号解锁代理。
- anyTLS 防的是「**审查者被动观察流量,通过包长/时序统计识别出「TLS 里套着 TLS」**」。所以它的核心是:用 PaddingScheme 把内层 TLS 握手包拆散、填充,让长度特征对不上已知指纹。

**2. 是否产生 TLS 嵌套,是两者最根本的结构差异。**
- anyTLS = 外层标准 TLS(自签/自有证书)+ 内层被代理的 TLS。两层都是真 TLS,**必然产生 TLS-in-TLS 嵌套**,所以才需要 PaddingScheme 去「打散」内层特征。FAQ 也承认这种嵌套开销(超 MTU、小包缺失、多握手往返)无法根本消除,除非 MITM 破坏端到端加密。
- REALITY 握手后**直接用 ECDH 派生的 AEAD 加密应用数据,伪装成 TLS ApplicationData**,被代理流量直接走这层——**根本没有「第二层 TLS」**,所以天然不存在 TLS-in-TLS 问题,也就不需要 padding 来对抗它。REALITY 要对抗的是握手阶段的主动探测。

**3. 对「特征写死」的态度不同。** anyTLS 的 FAQ 专门用一段模拟 XTLS-Vision 的 PaddingScheme(`stop=3, 0~2=900-1400`)来点评 Vision:「写死的长度处理逻辑,只要 GFW 更新特征库就能识别」;anyTLS 的对策是让 padding 方案**服务端可动态下发更新**,把「特征易变」作为设计目标。

---

## 五、关系:可组合而非互斥

两者层面不同,可以叠加:
- `ssrlive/reality-rs` 的 **AnyReality = REALITY-wrapped AnyTLS**:用 REALITY 解决「主动探测 + 免域名证书」,用 anyTLS 解决「内层多路复用 + 残留的流量特征」。
- 但需注意:REALITY 本身已通过「劫持握手 + 自建 AEAD」规避了双层 TLS 嵌套,所以 REALITY + VLESS/Vision 这条路线**天然没有 anyTLS 要解决的 TLS-in-TLS 问题**;anyTLS 的价值更多体现在「标准 TLS 栈 + 自带多路复用 + 可更新 padding」这套组合里。

---

## 参考来源

- anytls/anytls-go 协议说明 `docs/protocol.md`:https://github.com/anytls/anytls-go/blob/main/docs/protocol.md
- anytls/anytls-go FAQ `docs/faq.md`:https://github.com/anytls/anytls-go/blob/main/docs/faq.md
- anytls/anytls-go `proxy/padding/padding.go`:https://github.com/anytls/anytls-go/blob/main/proxy/padding/padding.go
- ssrlive/anytls-rs(描述 anyTLS 目标为缓解 TLS-in-TLS):https://github.com/ssrlive/anytls-rs
- ssrlive/reality-rs(AnyReality = REALITY-wrapped AnyTLS):https://github.com/ssrlive/reality-rs
- 本地 REALITY 实现:`transport/internet/reality/reality.go`(Xray-core 仓库),外部核心在依赖 `github.com/xtls/reality`
