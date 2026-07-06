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

## 六、私有抗审查方案设计

在上述三个方案(REALITY / anyTLS / Vision)的基础上,设计一套私有的抗审查方案。本节给出推荐设计与取舍。

### 6.1 "私有"的真正护城河

「自己写个协议、规范不公开」并非真正的护城河——客户端二进制一旦被审查方获取并逆向,所有硬编码参数(dest、公钥、padding 长度、uTLS 指纹)全部暴露,等于公开协议。anyTLS FAQ 对 Vision「写死的长度逻辑,GFW 更新特征库就能识别」的点评,说的就是这个问题。

因此私有方案的护城河应建立在两层:

1. **关键参数动态化**:服务端通过隧道持续下发新参数,客户端只是某一时刻的快照;
2. **客户端不公开分发**:限制逆向样本获取。

「协议规范保密」只是第三层、最弱的护城河。后续设计围绕前两层展开。

### 6.2 威胁模型

抗审查面对的攻击层次,按代价从低到高:

| 层 | 攻击 | 谁来防 |
|---|---|---|
| L1 | 被动 DPI(握手/包长/时序指纹) | 协议层 |
| L2 | 主动探测(审查者连上来验证) | 协议层 |
| L3 | 流量统计(包长分布、时长、带宽) | 整形层 |
| L4 | IP/端口封锁 | 部署层(多 IP、CDN) |
| L5 | 主动 MITM / QoS 限速 | 证书+抗抖动 |

协议设计主要管 L1–L3。**L4 是协议救不了的**——再好的协议 IP 被针对就得换 IP,这点必须先承认,避免在协议层做无用功。

### 6.3 设计原则

> **以 REALITY 为骨架(免域名 + 反主动探测 + 无 TLS 嵌套),以 Vision 为肌肉(握手期 padding + 稳态 direct-copy),以 anyTLS 的可下发 PaddingScheme 为皮肤(持续轻量整形),以"参数动态化"为免疫系统。**

每个部件都解决前面方案里另一个方案的弱点,不重复造轮子。

### 6.4 分层设计

#### 握手层:REALITY 骨架 + 4 处私有化改造

保留 REALITY 的核心:借真实 dest 的 TLS 握手外壳,认证暗号藏在 ClientHello 的 SessionId 里(AEAD 加密),握手后劫持连接走自建 AEAD。这给了三样东西:免域名/证书、反主动探测、无双层 TLS 嵌套(从根本上绕开 anyTLS 的核心痛点)。

私有化改造 4 点:

1. **dest 动态池,而非单一 dest**。REALITY 配一个静态 `microsoft.com`,被针对就全挂。私有版:服务端持有多个 dest 候选(选高流量、TLS1.3、HTTP/2、CDN 化的站),客户端连接时随机选,且通过隧道定期更新池。审查者难以枚举,单 dest 失效不影响整体。
2. **密钥轮换**。REALITY 的 PublicKey 静态。私有版:多 keyId + 多密钥对,定期通过隧道下发新公钥、旧密钥过期。私钥泄露的危害是审查者能解出暗号识别真假客户端,轮换把这个窗口缩到最短。
3. **暗号强时效 + 防重放**。REALITY 的 SessionId 里已有时间戳,但服务端校验较松。私有版:收紧到 ±5 分钟窗口,服务端记录已用时间戳 nonce 拒绝重放。这样审查者即使抓到一条合法握手包也无法重放探测。
4. **uTLS 指纹轮换**。在多个自洽的真实浏览器指纹(Chrome/Firefox/Safari/iOS)间轮换。注意必须用"自洽指纹"(整组 ClientHello 字段匹配某真实浏览器),不能拼凑——拼凑本身就是一个新指纹。

> 不要做的事:别去把暗号藏到"非标准字段"里搞隐写。SessionId 是 32 字节随机,AEAD 加密后仍是随机,审查者无法仅凭"SessionId 非空"判定代理(TLS1.3 legacy_session_id 本来就常见)。把暗号挪到更奇怪的地方反而增加实现风险、可能破坏 uTLS 指纹。位置固定不是问题,内容加密+动态化才是。

#### 传输层:Vision 风格 + 双向 + 可下发

REALITY 握手后是裸 AEAD ApplicationData,没有整形。这是它相对 anyTLS 的短板。被代理流量的真实包长会"透"过外层 AEAD 表现出来(AEAD 不改包长,只加密内容)。所以需要整形。

借鉴 Vision + anyTLS,但修正两者弱点:

- **握手期(Vision 路线)**:前 N 个包 padding 打散内层握手特征,检测到稳态 TLS 后切 direct-copy 零拷贝(性能)。修正 Vision 的"长度写死"——padding 长度方案改为可下发。
- **稳态期(轻量整形)**:不学 anyTLS 只管前 N 包。稳态期做两件低成本事:
  - 小包合并/填充(交互式 SSH 的小包是最易识别的特征,合并成中等尺寸);
  - 周期性注入少量 padding 包(cmdWaste 等价),打破包间隔的规律性。
  - 不对大流量满 MTU 包做处理(那是正常下载的特征,处理反而损失吞吐且无收益)。
- **双向**(修正 anyTLS 自陈"未处理下行"的弱点):上下行都整形,但下行可以更轻(下行大多是服务器→客户端的大流量,特征弱)。
- **PaddingScheme 可下发**(anyTLS 的精华):服务端通过隧道下发方案,且可基于观测到的流量统计自适应调整——这是"动态化"在整形层的落地。

#### 多路复用:默认关,可配开

anyTLS 内置 smux 强制复用。本方案反过来:**默认不复用,每条用户流独立握手**。理由:

- REALITY 握手只要 1 RTT,不复用的延迟代价可控。
- 长寿大流量的单连接是流量分析的甜点(一条连接跑几小时、几个 G,比短连接可疑得多)。
- 短连接更像正常 HTTPS 浏览行为,容易混入 dest 网站的背景流量。

如果延迟敏感(如多人共用、交互频繁),再开"有限复用":单连接最多 2–5 个流,且每流独立 padding,连接达阈值主动关闭重建。

#### 动态化层:私有方案的核心

这是把前面所有"可下发"汇聚成一个机制。在隧道内开一个 **control stream**,服务端定期 push:

```
dest_pool:        [microsoft.com, apple.com, ...]   # dest 候选
padding_scheme:   <新方案>                          # 整形方案
server_keys:      [{keyId, pubKey, expiry}]          # 公钥轮换
fingerprint_pool: [Chrome-120, Firefox-...]          # 指纹轮换
short_id_seed:    <新种子>                            # 暗号种子
```

客户端本地缓存,下次连接使用。即使客户端被逆向,审查者拿到的只是过期快照。这是私有方案区别于公开方案的本质——公开方案的参数是规范的一部分(静态),本方案的参数是运行时数据(动态)。

实现上,control stream 本身要轻量、低频(如每 30 分钟一次或事件触发),避免它自己变成新特征。

#### 反主动探测:保留透传 + 加"伸缩"

保留 REALITY 的透传(认证不过→透明转发给真实 dest,探测者看到合法网站)。再加一个私有增强:**伸缩性**——服务端检测到探测压力升高(短时间大量无暗号连接、来自已知探测 IP 段),自动短暂"完全伪装"(全部透传、拒绝代理功能),过一段时间再恢复。让审查者无法稳定判定"这是代理站还是镜像站",探测成本不可预测。

### 6.5 推荐架构(分层)

```
用户流量
   │
   ▼
┌──────────────────────────────────────────────────┐
│ 应用层:VLESS(精简) / 或自定义最小 framing         │
├──────────────────────────────────────────────────┤
│ 整形层:握手期 padding + 稳态轻量整形(双向,可下发)  │  ← Vision + anyTLS 精华
├──────────────────────────────────────────────────┤
│ 加密层:ECDH 派生 AEAD,伪装 TLS AppData(无嵌套)    │  ← REALITY
├──────────────────────────────────────────────────┤
│ 握手层:REALITY 借壳 + dest池/密钥轮换/暗号时效      │  ← 私有化改造
├──────────────────────────────────────────────────┤
│ 传输层:TCP(可叠 CDN 前置)                         │
└──────────────────────────────────────────────────┘
        ▲
        │ control stream(隧道内下发动态参数)
┌───────┴──────────────────────────────────────────┐
│ 动态化层:dest/key/padding/fingerprint 管理        │  ← 私有方案护城河
└──────────────────────────────────────────────────┘
```

### 6.6 在 Xray-core 上的实现路径

这套方案能在 Xray-core 现有架构上渐进实现,不必从零写:

- **握手层**:改 `transport/internet/reality/reality.go` + fork 外部依赖 `github.com/xtls/reality`(dest 池、密钥轮换、暗号时效校验都在这里)。
- **整形层**:在 `transport/internet/` 新增一个 `shaper`(借鉴 `anytls-go/proxy/padding` 的 `GenerateRecordPayloadSizes` 思路,但做成 transport 级、双向、可下发)。和 `proxy/proxy.go` 的 `VisionReader/Writer` 协同——握手期走 Vision,稳态期走 shaper。
- **动态化层**:新增一个 `app/dynconfig`(参考 `app/observatory` 的 app 注册模式),维护 dest/key/padding 池;通过隧道内 control stream 下发(可在 VLESS 之上挂一个轻量协议,或复用 `common/mux` 的控制通道)。
- **配置**:新增 `infra/conf` 的 JSON 结构 + `Build()`,按本仓库"添加协议/传输"的 3-touchpoint 模式接入(JSON struct + `Build()` + `common.RegisterConfig` + `main/distro/all` blank import)。

工作量大头在握手层 fork 和 shaper,其余是接线。

### 6.7 避坑清单

前面方案的教训:

1. **不要自造加密原语**。用标准 ECDH/AEAD/HKDF。审查者不靠破解加密识别代理,自造加密只有风险没有收益。
2. **不要写死任何特征**。Vision 的教训。所有可变参数走 control stream。
3. **不要走 anyTLS 的"自签证书 + 自己域名"路线做明面**。这给了审查者明确的封禁目标(域名/IP)。REALITY 路线免域名是质的优势,别放弃。
4. **不要忽视下行**。anyTLS 自陈的弱点。双向整形。
5. **不要追求完美 traffic morphing**。数学上不可检测做不到,目标是"把审查成本抬到他不愿付",够用即停。过度整形毁吞吐。
6. **不要把"私有"押在规范保密上**。重心放动态化 + 客户端不公开分发。
7. **不要在协议层解决 IP 封锁**。多 IP / CDN 前置是部署层的事,协议只能配合(如支持多 server entry)。

### 6.8 能力边界

- **L1–L2(被动 DPI + 主动探测)**:这套方案能挡住,且是 REALITY 已验证的强项。
- **L3(长期流量统计)**:能显著提高成本,但不能保证不被发现。审查方若有足够强的流量分类模型 + 长期观测,仍可能识别。整形的目标是"让成本高于收益",不是"数学不可检测"。
- **L4(IP 封锁)**:协议层无解。必须配多 IP / 域名前置 / CDN,且 server entry 要能快速切换(control stream 可以顺带下发新 entry)。
- **客户端逆向**:动态化能限制泄露窗口,但如果客户端持续泄露最新参数(如被持续运行并抓包),动态化也会被跟踪。最终仍依赖"客户端不公开分发"这个运维约束。

**一句话总结**:这套方案的实质是「REALITY 的握手哲学(借壳 + 反探测 + 无嵌套)+ 全参数动态化」。前者是公开就能用的最强地基,后者才是"私有"二字的真正价值——不公开的不是协议,而是会变的参数。

---

## 参考来源

- anytls/anytls-go 协议说明 `docs/protocol.md`:https://github.com/anytls/anytls-go/blob/main/docs/protocol.md
- anytls/anytls-go FAQ `docs/faq.md`:https://github.com/anytls/anytls-go/blob/main/docs/faq.md
- anytls/anytls-go `proxy/padding/padding.go`:https://github.com/anytls/anytls-go/blob/main/proxy/padding/padding.go
- ssrlive/anytls-rs(描述 anyTLS 目标为缓解 TLS-in-TLS):https://github.com/ssrlive/anytls-rs
- ssrlive/reality-rs(AnyReality = REALITY-wrapped AnyTLS):https://github.com/ssrlive/reality-rs
- 本地 REALITY 实现:`transport/internet/reality/reality.go`(Xray-core 仓库),外部核心在依赖 `github.com/xtls/reality`
