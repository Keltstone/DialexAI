@file:Suppress("DEPRECATION")

package com.dialex.presentation.settings

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.DisableSelection
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.automirrored.outlined.FactCheck
import androidx.compose.material.icons.automirrored.outlined.OpenInNew
import androidx.compose.material.icons.outlined.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.dialex.theme.LocalCcColors
import com.dialex.ui.DialexLogoView
import com.dialex.ui.KoltaLabsLogoView

private enum class AboutSubTab(val label: String, val icon: ImageVector) {
    OVERVIEW("Product Overview", Icons.Outlined.Info),
    SCENARIOS("Enterprise Scenarios", Icons.Outlined.Lightbulb),
    PRIVACY("Privacy", Icons.Outlined.Security),
    LICENSE("License (PolyForm)", Icons.Outlined.Gavel),
    TERMS("Terms & Advisory", Icons.Outlined.Description)
}

/**
 * Enterprise-grade About & Governance screen for Dialex.
 * Maintained and sponsored by Kolta Labs.
 * Features responsive adaptive layout that reflows pills and sub-tabs cleanly
 * across arbitrary window widths without text clipping or horizontal overflow.
 */
@Composable
fun AboutTab(
    connectionLabel: String,
    consent: com.dialex.domain.model.LegalConsent? = null,
    onShowFeedback: (() -> Unit)? = null,
    onOpenLicensingDedication: (() -> Unit)? = null
) {
    val cc = LocalCcColors.current
    val uriHandler = LocalUriHandler.current
    val clipboardManager = LocalClipboardManager.current
    var selectedSubTab by remember { mutableStateOf(AboutSubTab.OVERVIEW) }
    var copiedLabel by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(copiedLabel) {
        if (copiedLabel != null) {
            kotlinx.coroutines.delay(2000)
            copiedLabel = null
        }
    }

    val scrollState = rememberScrollState()

    SelectionContainer {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(scrollState)
                .padding(bottom = 32.dp),
            verticalArrangement = Arrangement.spacedBy(18.dp)
        ) {
            // ── 1. Brand & Executive Identity Header ─────────────────────────────
            Surface(
                shape = RoundedCornerShape(14.dp),
                color = cc.panel,
                border = BorderStroke(1.dp, cc.border.copy(alpha = 0.55f)),
                modifier = Modifier.fillMaxWidth()
            ) {
            BoxWithConstraints(modifier = Modifier.fillMaxWidth().padding(20.dp)) {
                val isNarrow = maxWidth < 680.dp

                Column(
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(16.dp)
                    ) {
                        DialexLogoView(
                            size = if (isNarrow) 56.dp else 68.dp,
                            shape = RoundedCornerShape(14.dp)
                        )

                        Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            Text(
                                "Dialex AI",
                                style = MaterialTheme.typography.headlineMedium.copy(
                                    fontWeight = FontWeight.Bold,
                                    fontSize = if (isNarrow) 22.sp else 26.sp
                                ),
                                color = cc.textPrimary
                            )

                            Text(
                                "Autonomous Multi-Agent Deliberation & Decision Intelligence Council",
                                style = MaterialTheme.typography.bodyMedium.copy(fontSize = 13.sp, fontWeight = FontWeight.Medium),
                                color = cc.textPrimary.copy(alpha = 0.9f)
                            )

                            Text(
                                "Peer-reviewed consensus engineering for high-consequence technical, architectural, and policy decisions.",
                                style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                                color = cc.textMuted
                            )
                        }
                    }

                    Row(
                        modifier = Modifier.padding(top = 16.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        Text(
                            "Version 1.0.0",
                            style = MaterialTheme.typography.bodySmall.copy(
                                fontSize = 12.sp,
                                fontWeight = FontWeight.Normal
                            ),
                            color = cc.textMuted
                        )

                        if (onOpenLicensingDedication != null) {
                            Text(
                                "•",
                                style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.sp),
                                color = cc.textMuted.copy(alpha = 0.4f)
                            )
                            Text(
                                "Licensing Dedication & Notices",
                                style = MaterialTheme.typography.bodySmall.copy(
                                    fontSize = 11.sp,
                                    fontWeight = FontWeight.Normal,
                                    color = cc.textMuted.copy(alpha = 0.75f)
                                ),
                                modifier = Modifier
                                    .clip(RoundedCornerShape(4.dp))
                                    .clickable { onOpenLicensingDedication() }
                                    .padding(horizontal = 4.dp, vertical = 2.dp)
                            )
                        }

                        if (consent != null && consent.isAccepted) {
                            Surface(
                                shape = RoundedCornerShape(4.dp),
                                color = Color(0xFF10B981).copy(alpha = 0.12f),
                                border = BorderStroke(0.75.dp, Color(0xFF10B981).copy(alpha = 0.35f))
                            ) {
                                Row(
                                    modifier = Modifier.padding(horizontal = 7.dp, vertical = 2.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                                ) {
                                    Icon(Icons.Outlined.CheckCircle, contentDescription = null, tint = Color(0xFF10B981), modifier = Modifier.size(11.dp))
                                    Text(
                                        "Terms & Privacy Accepted (v${consent.termsVersion})",
                                        style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp, fontWeight = FontWeight.Medium),
                                        color = Color(0xFF10B981)
                                    )
                                }
                            }
                        }
                    }
                }
            }
        }

        // ── 2. Navigation Sub-Tabs ───────────────────────────────────────────
        DisableSelection {
            Surface(
                shape = RoundedCornerShape(10.dp),
                color = cc.panelAlt,
                border = BorderStroke(0.85.dp, cc.border.copy(alpha = 0.45f)),
                modifier = Modifier.fillMaxWidth()
            ) {
                BoxWithConstraints(modifier = Modifier.fillMaxWidth().padding(4.dp)) {
                    val isNarrow = maxWidth < 560.dp
                    if (isNarrow) {
                        Column(
                            modifier = Modifier.fillMaxWidth(),
                            verticalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            AboutSubTab.entries.forEach { subTab ->
                                SubTabItem(
                                    subTab = subTab,
                                    isSelected = selectedSubTab == subTab,
                                    cc = cc,
                                    modifier = Modifier.fillMaxWidth()
                                ) {
                                    selectedSubTab = subTab
                                }
                            }
                        }
                    } else {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            AboutSubTab.entries.forEach { subTab ->
                                SubTabItem(
                                    subTab = subTab,
                                    isSelected = selectedSubTab == subTab,
                                    cc = cc,
                                    modifier = Modifier.weight(1f)
                                ) {
                                    selectedSubTab = subTab
                                }
                            }
                        }
                    }
                }
            }
        }

        // ── 3. Tab Content Switcher ──────────────────────────────────────────
        AnimatedContent(
            targetState = selectedSubTab,
            transitionSpec = { fadeIn() togetherWith fadeOut() },
            modifier = Modifier.fillMaxWidth()
        ) { tab ->
            when (tab) {
                AboutSubTab.OVERVIEW -> OverviewSection(cc, uriHandler, onShowFeedback, onOpenLicensingDedication)
                AboutSubTab.SCENARIOS -> ScenariosSection(cc)
                AboutSubTab.PRIVACY -> PrivacySection(cc, clipboardManager) { copiedLabel = it }
                AboutSubTab.LICENSE -> LicenseSection(cc, clipboardManager, onOpenLicensingDedication) { copiedLabel = it }
                AboutSubTab.TERMS -> TermsSection(cc, clipboardManager) { copiedLabel = it }
            }
        }
    }
}
}

@Composable
private fun SubTabItem(
    subTab: AboutSubTab,
    isSelected: Boolean,
    cc: com.dialex.theme.CcPalette,
    modifier: Modifier = Modifier,
    onClick: () -> Unit
) {
    val tabBg = if (isSelected) cc.panel else Color.Transparent
    val tabBorder = if (isSelected) cc.border.copy(alpha = 0.5f) else Color.Transparent
    val textColor = if (isSelected) cc.textPrimary else cc.textMuted

    Surface(
        shape = RoundedCornerShape(7.dp),
        color = tabBg,
        border = BorderStroke(if (isSelected) 0.85.dp else 0.dp, tabBorder),
        modifier = modifier
            .clip(RoundedCornerShape(7.dp))
            .clickable(onClick = onClick)
    ) {
        Row(
            modifier = Modifier.padding(vertical = 8.dp, horizontal = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.Center
        ) {
            Icon(
                subTab.icon,
                contentDescription = null,
                modifier = Modifier.size(14.dp),
                tint = if (isSelected) cc.accent else cc.textMuted
            )
            Spacer(Modifier.width(6.dp))
            Text(
                subTab.label,
                style = MaterialTheme.typography.labelMedium.copy(
                    fontSize = 11.5.sp,
                    fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal
                ),
                color = textColor,
                maxLines = 1
            )
        }
    }
}
// Sub-Tab 1: Product Overview (What is Dialex)
@Composable
private fun OverviewSection(
    cc: com.dialex.theme.CcPalette,
    uriHandler: androidx.compose.ui.platform.UriHandler,
    onShowFeedback: (() -> Unit)?,
    onOpenLicensingDedication: (() -> Unit)? = null
) {
    Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
        // Section: Executive Summary & The Problem
        SettingCard {
            Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                SectionHeader(
                    title = "The Foundation: Overcoming Single-Model Blind Spots",
                    subtitle = "Eliminating sycophancy and unverified inferences in high-consequence engineering"
                )

                Text(
                    "Large language models evaluated in isolation present fundamental vulnerabilities in technical and strategic domains. When queried individually, even frontier models exhibit cognitive sycophancy—reinforcing flawed user premises rather than correcting them—as well as confirmation bias and hallucinations. Engineering teams attempting to resolve these gaps by manually copying prompts across separate browser tabs face fragmentation, context loss, and slow decision velocity.",
                    style = MaterialTheme.typography.bodyMedium.copy(fontSize = 13.sp, lineHeight = 21.sp),
                    color = cc.textPrimary
                )

                Text(
                    "Dialex transforms isolated model queries into a structured, asynchronous deliberation council. By seating distinct frontier models—such as Anthropic Claude, OpenAI GPT, Google Gemini, xAI Grok, DeepSeek, and Mistral—into a multi-agent roundtable, Dialex subjects every proposal to rigorous adversarial cross-examination. Models inspect one another's reasoning, contest edge cases, and negotiate until a mathematically verified consensus threshold is satisfied.",
                    style = MaterialTheme.typography.bodyMedium.copy(fontSize = 13.sp, lineHeight = 21.sp),
                    color = cc.textPrimary
                )
            }
        }

        // Section: System Architecture & Operational Pillars
        SettingCard {
            Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                SectionHeader(
                    title = "System Architecture & Operational Pillars",
                    subtitle = "Five foundational engineering principles guaranteeing determinism, operational sovereignty, and economic efficiency"
                )

                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    OperationalPillarRow(
                        pillarNumber = "01",
                        icon = Icons.Outlined.Gavel,
                        title = "Autonomous Socratic Deliberation & Convergence Calculus",
                        tagline = "Eliminating single-model bias through structured adversarial peer review",
                        description = "Dialex orchestrates multi-agent deliberations using a sequential turn protocol governed by explicit epistemological constraints. Rather than generating uncoordinated parallel outputs, participant agents actively scrutinize, validate, and dispute claims made in preceding turns. The engine monitors objection resolution vectors and semantic delta metrics in real time, automatically determining true consensus convergence and halting execution once mathematical stability is reached.",
                        highlights = listOf("Sequenced Turn Orchestration", "Unresolved Objection Graph", "Automated Convergence Detection"),
                        cc = cc
                    )

                    OperationalPillarRow(
                        pillarNumber = "02",
                        icon = Icons.Outlined.AssignmentTurnedIn,
                        title = "Deterministic Synthesis & Executive Deliverables",
                        tagline = "Transforming complex technical debates into verifiable work products",
                        description = "Deliberation transcripts are synthesized deterministically into standardized, executive-grade artifacts—including Architectural Decision Records (ADRs), STRIDE Threat Modeling Matrices, blameless post-mortem documents, and risk assessment matrices. Unlike conventional summarization which flattens controversy, Dialex preserves dissenting opinions, minority failure-mode analyses, and explicit rollback triggers.",
                        highlights = listOf("Formal ADR Generation", "STRIDE Threat Modeling", "Dissenting Opinion Preservation"),
                        cc = cc
                    )

                    OperationalPillarRow(
                        pillarNumber = "03",
                        icon = Icons.Outlined.Dns,
                        title = "High-Performance Local Go Daemon & Loopback IPC",
                        tagline = "Sub-millisecond local execution with zero cloud transit or proxying",
                        description = "The core engine runs as an embedded or background native Go daemon (dialex serve). The frontend connects via sub-millisecond localhost HTTP/2 REST and Server-Sent Events (SSE) streaming or Unix domain sockets. All state transitions, transcript branches, and configuration scopes are persisted atomically to local JSON storage with zero intermediary cloud proxies.",
                        highlights = listOf("Native Compiled Go Core", "Localhost IPC & SSE Streaming", "Atomic Filesystem Persistence"),
                        cc = cc
                    )

                    OperationalPillarRow(
                        pillarNumber = "04",
                        icon = Icons.Outlined.AttachMoney,
                        title = "Sovereign BYOK Economics & Native CLI Subscription Passthrough",
                        tagline = "Zero platform tolls, direct-to-provider routing, and flat-rate subscription leverage",
                        description = "Dialex operates with zero token margins and zero platform transaction markups. When using API keys, requests travel directly from the user's host machine to official provider endpoints. For teams with existing developer subscriptions, Dialex integrates natively with official CLI runners (such as Claude Code and OpenAI Codex), enabling flat-rate subscription utilization without incremental per-token API charges.",
                        highlights = listOf("Zero Intermediary Markup", "Direct-to-Provider Wire Protocol", "Native CLI Subscription Runners"),
                        cc = cc
                    )

                    OperationalPillarRow(
                        pillarNumber = "05",
                        icon = Icons.Outlined.Shield,
                        title = "Zero-Telemetry Mandate & Sovereign Air-Gapped Operation",
                        tagline = "Defense-grade privacy compliance with zero outbound metadata leakage",
                        description = "By architectural decree, Dialex contains zero tracking pixels, behavioral telemetry hooks, error harvesting beacons, or cloud analytics pingers. When paired with local foundation runtimes (such as Ollama or on-device llama.cpp weights), the entire system functions completely disconnected from the Internet—providing airtight protection for trade secrets, sensitive IP, and proprietary codebases.",
                        highlights = listOf("Zero Telemetry Beacons", "100% Offline Air-Gapped Capable", "Cryptographic Keyring Protection"),
                        cc = cc
                    )
                }
            }
        }

        // Section: Kolta Labs (Privacy-Focused Developer Brand)
        SettingCard {
            Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(14.dp)
                    ) {
                        KoltaLabsLogoView(size = 40.dp)
                        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                            Text(
                                "Kolta Labs",
                                style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.Bold, fontSize = 18.sp),
                                color = cc.textPrimary
                            )
                            Text(
                                "Independent software engineered for autonomy, privacy, and local execution.",
                                style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                                color = cc.textMuted
                            )
                        }
                    }

                    DisableSelection {
                        OutlinedButton(
                            onClick = { uriHandler.openUri("https://github.com/Keltstone/DialexAI") },
                            shape = RoundedCornerShape(6.dp),
                            modifier = Modifier.height(28.dp),
                            contentPadding = PaddingValues(horizontal = 8.dp)
                        ) {
                            Icon(Icons.AutoMirrored.Outlined.OpenInNew, contentDescription = null, modifier = Modifier.size(11.dp), tint = cc.textMuted)
                            Spacer(Modifier.width(4.dp))
                            Text("Source", style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp), color = cc.textMuted)
                        }
                    }
                }

                HorizontalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.75.dp)

                Text(
                    "Modern software tooling has largely drifted toward centralized cloud proxies, mandatory accounts, and continuous telemetry. While convenient for software vendors, this model introduces needless security exposure, vendor lock-in, and unpredictable platform dependencies for engineering teams.",
                    style = MaterialTheme.typography.bodyMedium.copy(fontSize = 12.5.sp, lineHeight = 19.sp),
                    color = cc.textPrimary
                )

                Text(
                    "Kolta Labs was started to take a different architectural path: building tools that run entirely on local machines, keep credentials under the developer's sole custody, and treat your workstation as sovereign ground. Dialex is designed from first principles so that project files, debate transcripts, and API traffic travel directly between your machine and your chosen model providers—with zero intermediary servers, zero platform markups, and zero background analytics.",
                    style = MaterialTheme.typography.bodyMedium.copy(fontSize = 12.5.sp, lineHeight = 19.sp),
                    color = cc.textPrimary
                )


                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(8.dp))
                        .background(cc.panelAlt)
                        .padding(12.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    SpecRow("Project Maintainer", "Kolta Labs", cc)
                    SpecRow("Software License", "PolyForm Noncommercial License 1.0.0", cc)
                    SpecRow("Source Code", "github.com/Keltstone/DialexAI", cc)
                }
            }
        }

        // Section: Technical Specifications
        SettingCard {
            Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                SectionHeader(
                    title = "Technical Specifications & Environment",
                    subtitle = "Dialex application runtime and deployment configuration"
                )

                Column(
                    modifier = Modifier.fillMaxWidth(),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    SpecRow("Application Framework", "Kotlin Multiplatform (KMP) & Compose Multiplatform 1.10", cc)
                    SpecRow("Core Daemon Engine", "Dialex Native Go Daemon (embedded or background service)", cc)
                    SpecRow("Communication Protocol", "Localhost HTTP/2 REST & Server-Sent Events (SSE) Stream", cc)
                    SpecRow("Execution Environments", "Desktop (macOS, Linux, Windows) & Android on-device engine", cc)
                    SpecRow("Supported Model Providers", "Anthropic, OpenAI, Google Gemini, xAI Grok, DeepSeek, Mistral, Ollama (Local)", cc)
                    SpecRow("Data Persistence", "Local filesystem JSON storage (~/Library/Application Support/Dialex)", cc)
                    SpecRow("Licensing", "PolyForm Noncommercial License 1.0.0", cc)
                }

                Spacer(Modifier.height(4.dp))

                DisableSelection {
                    FlowRow(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        if (onShowFeedback != null) {
                            OutlinedButton(
                                onClick = onShowFeedback,
                                shape = RoundedCornerShape(8.dp),
                                modifier = Modifier.height(34.dp),
                                contentPadding = PaddingValues(horizontal = 12.dp)
                            ) {
                                Icon(Icons.Outlined.Forum, contentDescription = null, modifier = Modifier.size(13.dp), tint = cc.textPrimary)
                                Spacer(Modifier.width(6.dp))
                                Text("Submit Feedback", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp), color = cc.textPrimary)
                            }
                        }

                        OutlinedButton(
                            onClick = { uriHandler.openUri("https://github.com/Keltstone/DialexAI/issues") },
                            shape = RoundedCornerShape(8.dp),
                            modifier = Modifier.height(34.dp),
                            contentPadding = PaddingValues(horizontal = 12.dp)
                        ) {
                            Icon(Icons.Outlined.BugReport, contentDescription = null, modifier = Modifier.size(13.dp), tint = cc.textPrimary)
                            Spacer(Modifier.width(6.dp))
                            Text("Report an Issue", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp), color = cc.textPrimary)
                        }

                        OutlinedButton(
                            onClick = { uriHandler.openUri("https://github.com/Keltstone/DialexAI") },
                            shape = RoundedCornerShape(8.dp),
                            modifier = Modifier.height(34.dp),
                            contentPadding = PaddingValues(horizontal = 12.dp)
                        ) {
                            Icon(Icons.AutoMirrored.Outlined.OpenInNew, contentDescription = null, modifier = Modifier.size(13.dp), tint = cc.textMuted)
                            Spacer(Modifier.width(6.dp))
                            Text("Source Repository", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp), color = cc.textMuted)
                        }
                    }

                    if (onOpenLicensingDedication != null) {
                        Spacer(Modifier.height(8.dp))
                        HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)
                        Spacer(Modifier.height(4.dp))
                        Text(
                            "Statutory Licensing Dedication & Third-Party Credits",
                            style = MaterialTheme.typography.bodySmall.copy(
                                fontSize = 11.sp,
                                color = cc.textMuted.copy(alpha = 0.65f)
                            ),
                            modifier = Modifier
                                .clip(RoundedCornerShape(4.dp))
                                .clickable { onOpenLicensingDedication() }
                                .padding(vertical = 3.dp)
                        )
                    }
                }
            }
        }
    }
}
// Sub-Tab 2: Enterprise Scenarios (Deep Dive)
@Composable
private fun ScenariosSection(cc: com.dialex.theme.CcPalette) {
    Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
        SettingCard {
            Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                SectionHeader(
                    title = "Production Use Cases & Deployment Scenarios",
                    subtitle = "Where autonomous multi-model deliberation delivers verified business value"
                )
                Text(
                    "Dialex is designed for decision environments where failure is expensive. Below are five proven operational scenarios where multi-agent debate outperforms conventional single-turn prompting and human consensus meetings.",
                    style = MaterialTheme.typography.bodyMedium.copy(fontSize = 13.sp, lineHeight = 20.sp),
                    color = cc.textPrimary
                )
            }
        }

        // Scenario 1: Architecture & Technology Selection
        ScenarioCard(
            number = "01",
            title = "High-Stakes Architecture & Technology Stack Evaluation",
            subtitle = "Selecting core databases, message brokers, and distributed frameworks with zero lock-in bias",
            challenge = "Selecting foundational infrastructure (e.g. DynamoDB vs CockroachDB, Kafka vs NATS, Flutter vs Kotlin Multiplatform) involves multi-year commitments. Single-model inquiries consistently recommend whichever technology was most prevalent in training data without auditing operational overhead or organizational constraints.",
            councilSetup = "Primary: Claude 3.7 Sonnet (Pragmatic Systems Architect) • Seat 2: GPT-5.6 (Devil's Advocate) • Seat 3: Gemini 3.7 Pro (Performance Specialist) • Seat 4: DeepSeek (Cost/FinOps Auditor)",
            dynamics = "The primary architect proposes an initial design. The Devil's Advocate systematically stress-tests cold starts, network partition behavior, and failover semantics. The Performance Specialist benchmarks query latency under load, while the FinOps Auditor flags hidden IOPS and inter-region data egress tariffs. Once cross-model objections are addressed, the debate converges.",
            deliverable = "An Architectural Decision Record (ADR) detailing Context, Decision Drivers, Evaluated Options, Quantitative Trade-Off Matrix, Selected Architecture, and Explicit Rollback Triggers.",
            icon = Icons.Outlined.Hub,
            cc = cc
        )

        // Scenario 2: Threat Modeling & Security Review
        ScenarioCard(
            number = "02",
            title = "Zero-Trust Threat Modeling & Vulnerability Surface Audit",
            subtitle = "Adversarial STRIDE review of authentication flows, cryptographic protocols, and API boundaries",
            challenge = "Security architects often struggle to foresee complex multi-step attack vectors across distributed microservices. Single AI audits frequently produce superficial checklists that miss subtle token reuse, replay vulnerabilities, or privilege escalation paths.",
            councilSetup = "Primary: Claude (Security Auditor) • Seat 2: GPT (Red Team Attacker) • Seat 3: Gemini (Cloud Infrastructure SecOps)",
            dynamics = "The Red Team agent actively searches for exploitable entry points—probing token expiration edge cases, CORS misconfigurations, and unauthorized lateral movement paths. The Security Auditor proposes defense-in-depth mitigations (e.g. mutual TLS, ephemeral session keys, rate-limiting). The council debates the practicality of each defense against developer productivity.",
            deliverable = "A comprehensive STRIDE Threat Modeling Matrix with assigned CVSS 3.1 severity scores, confirmed attack vectors, and hardened cryptographic code snippets for immediate remediation.",
            icon = Icons.Outlined.Security,
            cc = cc
        )

        // Scenario 3: Incident Retrospectives (Post-Mortems)
        ScenarioCard(
            number = "03",
            title = "Blameless Post-Mortem & Incident Root-Cause Analysis",
            subtitle = "Synthesizing cascading distributed failures without cognitive bias or hindsight rationalization",
            challenge = "Following major service outages, incident reviews risk falling into hindsight bias, finger-pointing, or stopping at immediate symptoms rather than exposing systemic distributed flaws and alerting gaps.",
            councilSetup = "Primary: Gemini (Site Reliability Engineer) • Seat 2: Claude (Distributed Systems Debugger) • Seat 3: Mistral (Safety Inspector)",
            dynamics = "The council ingests incident logs, deployment timestamps, and metrics graphs. Agents cross-reference alerts against execution traces to isolate root triggers—distinguishing between database connection pool exhaustion, unhandled retry storms, and cascading circuit breaker failures. The council conducts an automated 5-Whys derivation.",
            deliverable = "An Executive Post-Incident Retrospective complete with detailed incident timeline, root cause analysis, contributing architectural factors, and a prioritized preventative backlog.",
            icon = Icons.Outlined.HistoryEdu,
            cc = cc
        )

        // Scenario 4: Technical Due Diligence & Vendor Auditing
        ScenarioCard(
            number = "04",
            title = "Vendor Selection, SDK Auditing & License Due Diligence",
            subtitle = "Evaluating open-source dependencies, enterprise SaaS, and proprietary cloud vendor lock-in",
            challenge = "Adopting third-party software introduces legal, financial, and operational risks—including viral copyleft contamination, aggressive telemetry tracking, deprecation velocity, and hidden subscription expansion clauses.",
            councilSetup = "Primary: GPT (Enterprise Enterprise Architect) • Seat 2: Claude (Open Source Legal Specialist) • Seat 3: DeepSeek (FinOps Analyst)",
            dynamics = "The legal specialist inspects dependency license trees for restrictive terms (e.g., SSPL, AGPL, BSL compliance). The enterprise architect evaluates SDK maintenance health, API stability, and migration complexity. The FinOps analyst calculates 3-year total cost of ownership (TCO) across projected volume growth.",
            deliverable = "A Vendor Evaluation Due Diligence Report featuring a risk heatmap, license compliance clearance, TCO projection, and vendor exit strategy specifications.",
            icon = Icons.AutoMirrored.Outlined.FactCheck,
            cc = cc
        )

        // Scenario 5: Regulatory Compliance & Governance
        ScenarioCard(
            number = "05",
            title = "Regulatory Compliance & Data Governance Certification",
            subtitle = "Auditing data flows, retention schedules, and telemetry for GDPR, HIPAA, and SOC 2 Type II readiness",
            challenge = "Demonstrating compliance across disparate global privacy frameworks requires exhaustive tracing of data ingestion, encryption, sub-processor transfers, and data subject deletion mechanisms.",
            councilSetup = "Primary: Claude (Regulatory Compliance Officer) • Seat 2: Gemini (Data Governance Architect) • Seat 3: GPT (Audit Specialist)",
            dynamics = "Agents audit project data pipelines against statutory requirements. The council cross-examines storage schemas, identifies potential personal data (PII) leakage in debug logs, and establishes cryptographic erasure procedures satisfying GDPR Article 17 and HIPAA Security Rules.",
            deliverable = "A Regulatory Readiness Audit Report with gap analysis, compliance certification roadmap, data classification inventory, and mandatory remediation directives.",
            icon = Icons.Outlined.Policy,
            cc = cc
        )
    }
}
// Sub-Tab 3: Privacy Policy & Data Governance Charter
@Composable
private fun PrivacySection(
    cc: com.dialex.theme.CcPalette,
    clipboardManager: androidx.compose.ui.platform.ClipboardManager,
    onCopied: (String) -> Unit
) {
    Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
        SettingCard {
            Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    SectionHeader(
                        title = "Privacy Policy & Data Sovereignty Charter",
                        subtitle = "Effective Date: January 1, 2026 • Legal Standard v1.0 • Maintained by Kolta Labs"
                    )

                    DisableSelection {
                        OutlinedButton(
                            onClick = {
                                clipboardManager.setText(AnnotatedString(LegalTexts.FullPrivacyPolicy))
                                onCopied("Privacy Policy")
                            },
                            shape = RoundedCornerShape(6.dp),
                            modifier = Modifier.height(28.dp),
                            contentPadding = PaddingValues(horizontal = 8.dp)
                        ) {
                            Icon(Icons.Outlined.ContentCopy, contentDescription = null, modifier = Modifier.size(12.dp), tint = cc.textMuted)
                            Spacer(Modifier.width(4.dp))
                            Text("Copy Full Charter", style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp), color = cc.textMuted)
                        }
                    }
                }

                HorizontalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.75.dp)

                LegalParagraph(
                    title = "1. Absolute Zero-Telemetry Warranty",
                    content = "Dialex is architected by Kolta Labs from the ground up as a zero-telemetry system. The software does not collect, record, harvest, profile, or transmit user analytics, tracking pixels, crash identifiers, prompt logs, or behavioral telemetry to Kolta Labs maintainers or any third-party analytics vendor. Dialex operates no intermediary cloud servers, proxy networks, or central collection databases.",
                    cc = cc
                )

                LegalParagraph(
                    title = "2. Local-First Data Residence & Custody",
                    content = "All deliberation data—including discussion prompts, attached files, persona definitions, project workspace scopes, debate transcripts, and generated deliverables—resides exclusively on your local host filesystem. You retain sole, unencumbered custody and ownership of your data at all times. Data files are stored in open, transparent JSON structures within your standard application data directory and are never uploaded or synced to external cloud repositories without your explicit, voluntary action.",
                    cc = cc
                )

                LegalParagraph(
                    title = "3. Direct-to-Provider Transport & No-Interception Guarantee",
                    content = "When configured to interact with cloud foundation models (including Anthropic, OpenAI, Google, xAI, and Mistral), network requests originate directly from your local machine to the official, verified TLS/HTTPS API endpoints of the respective provider. Dialex does not proxy, intercept, store, or forward these payloads through intermediate servers. All interactions are subject strictly and solely to your direct commercial agreements with those providers.",
                    cc = cc
                )

                LegalParagraph(
                    title = "4. Complete Offline & Air-Gapped Operational Mode",
                    content = "When configured with local model providers (such as Ollama or custom local CLI executables), Dialex operates completely disconnected from the Internet. In this configuration, zero network packets leave your host device, making Dialex fully compliant with defense-grade air-gapped environments, proprietary codebase protections, and strict trade-secret isolation protocols.",
                    cc = cc
                )

                LegalParagraph(
                    title = "5. Cryptographic Credential Protection",
                    content = "API keys and sensitive authentication tokens entered into Dialex are stored locally on your device and encrypted using native operating system cryptographic primitives (such as the macOS Keychain or Linux Secret Service API). Secrets are accessed in-memory only during active, user-initiated model invocations and are never written to unencrypted log files.",
                    cc = cc
                )

                LegalParagraph(
                    title = "6. Immediate & Irreversible Data Purging",
                    content = "You maintain complete administrative authority over data retention. Triggering conversation deletion or project removal immediately and permanently purges the corresponding files from your local disk without tombstoning, delayed garbage collection, or hidden backup retention.",
                    cc = cc
                )
            }
        }
    }
}
// Sub-Tab 4: Software License Agreement
@Composable
private fun LicenseSection(
    cc: com.dialex.theme.CcPalette,
    clipboardManager: androidx.compose.ui.platform.ClipboardManager,
    onOpenLicensingDedication: (() -> Unit)? = null,
    onCopied: (String) -> Unit
) {
    Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
        SettingCard {
            Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    SectionHeader(
                        title = "Software License Agreement",
                        subtitle = "PolyForm Noncommercial License 1.0.0 • Free for Individuals • Kolta Labs"
                    )

                    DisableSelection {
                        OutlinedButton(
                            onClick = {
                                clipboardManager.setText(AnnotatedString(LegalTexts.PolyFormSummary))
                                onCopied("License Agreement")
                            },
                            shape = RoundedCornerShape(6.dp),
                            modifier = Modifier.height(28.dp),
                            contentPadding = PaddingValues(horizontal = 8.dp)
                        ) {
                            Icon(Icons.Outlined.ContentCopy, contentDescription = null, modifier = Modifier.size(12.dp), tint = cc.textMuted)
                            Spacer(Modifier.width(4.dp))
                            Text("Copy License Text", style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp), color = cc.textMuted)
                        }
                    }
                }

                HorizontalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.75.dp)

                // Key rights callout — responsive column or flow row
                BoxWithConstraints(modifier = Modifier.fillMaxWidth()) {
                    val isNarrow = maxWidth < 560.dp
                    if (isNarrow) {
                        Column(
                            modifier = Modifier.fillMaxWidth(),
                            verticalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            LicensePermissionPill("✔ Free for Individual & Noncommercial Use", Color(0xFF10B981), cc, Modifier.fillMaxWidth())
                            LicensePermissionPill("✔ Full Code Inspection & Modification", Color(0xFF10B981), cc, Modifier.fillMaxWidth())
                            LicensePermissionPill("🔒 Commercial Use Requires Enterprise License", Color(0xFFF59E0B), cc, Modifier.fillMaxWidth())
                        }
                    } else {
                        FlowRow(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            LicensePermissionPill("✔ Free for Individual & Noncommercial Use", Color(0xFF10B981), cc)
                            LicensePermissionPill("✔ Full Code Inspection & Modification", Color(0xFF10B981), cc)
                            LicensePermissionPill("🔒 Commercial Use Requires Enterprise License", Color(0xFFF59E0B), cc)
                        }
                    }
                }

                Text(
                    "Dialex is source-available software licensed under the PolyForm Noncommercial License 1.0.0. You are permitted to execute, inspect, modify, and redistribute Dialex for personal study, academic research, education, hobby, or noncommercial pursuits.",
                    style = MaterialTheme.typography.bodyMedium.copy(fontSize = 12.5.sp, lineHeight = 19.sp),
                    color = cc.textPrimary
                )

                LegalParagraph(
                    title = "Individual & Noncommercial Scope",
                    content = "Under the PolyForm Noncommercial License 1.0.0, personal use for research, experimentation, personal study, hobby projects, or use by noncommercial/educational organizations is permitted. Any use that involves using the software to earn revenue or run a commercial business directly or indirectly requires an agreement with Kolta Labs.",
                    cc = cc
                )

                LegalParagraph(
                    title = "Statutory Disclaimer of Warranty",
                    content = "AS FAR AS THE LAW ALLOWS, THE SOFTWARE COMES AS IS, WITHOUT ANY WARRANTY OR CONDITION, AND THE LICENSOR WILL NOT BE LIABLE TO YOU FOR ANY DAMAGES ARISING OUT OF THESE TERMS OR THE USE OR NATURE OF THE SOFTWARE, UNDER ANY KIND OF LEGAL CLAIM.",
                    cc = cc
                )

                // Monospace scrollable license summary
                Surface(
                    shape = RoundedCornerShape(8.dp),
                    color = if (cc.isDark) Color(0xFF14151B) else Color(0xFFF3F4F6),
                    border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.4f)),
                    modifier = Modifier.fillMaxWidth()
                ) {
                    SelectionContainer {
                        Text(
                            text = LegalTexts.PolyFormSummary,
                            style = MaterialTheme.typography.bodySmall.copy(
                                fontFamily = FontFamily.Monospace,
                                fontSize = 11.sp,
                                lineHeight = 16.5.sp
                            ),
                            color = cc.textMuted,
                            modifier = Modifier.padding(14.dp)
                        )
                    }
                }

                if (onOpenLicensingDedication != null) {
                    Spacer(Modifier.height(4.dp))
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(6.dp))
                            .clickable { onOpenLicensingDedication() }
                            .padding(vertical = 4.dp),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            "View Complete Statutory Licensing Dedication & Third-Party OSS Notices",
                            style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp, color = cc.accent, fontWeight = FontWeight.Medium)
                        )
                        Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, contentDescription = null, tint = cc.accent, modifier = Modifier.size(15.dp))
                    }
                }
            }
        }
    }
}
// Sub-Tab 5: Terms of Service & Advisory Disclaimer
@Composable
private fun TermsSection(
    cc: com.dialex.theme.CcPalette,
    clipboardManager: androidx.compose.ui.platform.ClipboardManager,
    onCopied: (String) -> Unit
) {
    Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
        SettingCard {
            Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    SectionHeader(
                        title = "Terms of Service & Advisory Notice",
                        subtitle = "Responsible AI Usage & Professional Advisory Bounds • Kolta Labs"
                    )

                    DisableSelection {
                        OutlinedButton(
                            onClick = {
                                clipboardManager.setText(AnnotatedString(LegalTexts.FullTermsOfService))
                                onCopied("Terms of Service")
                            },
                            shape = RoundedCornerShape(6.dp),
                            modifier = Modifier.height(28.dp),
                            contentPadding = PaddingValues(horizontal = 8.dp)
                        ) {
                            Icon(Icons.Outlined.ContentCopy, contentDescription = null, modifier = Modifier.size(12.dp), tint = cc.textMuted)
                            Spacer(Modifier.width(4.dp))
                            Text("Copy Terms", style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp), color = cc.textMuted)
                        }
                    }
                }

                HorizontalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.75.dp)

                LegalParagraph(
                    title = "1. Scope and Agreement",
                    content = "By downloading, installing, compiling, executing, or accessing Dialex, you agree to be bound by these Terms of Service. If you do not consent to these terms, you must terminate your use of the application and remove it from your devices immediately.",
                    cc = cc
                )

                LegalParagraph(
                    title = "2. Characterization of Non-Deterministic AI Deliberations",
                    content = "Dialex is an analytical orchestration framework that simulates structured debate between statistical machine learning models. All outputs—including debate messages, transcripts, ADRs, matrices, recommendations, risk scores, and generated code—represent synthetic, non-deterministic machine inferences. They do not constitute certified software engineering, legal, financial, architectural, or medical advice. Users bear ultimate responsibility for independently verifying, compiling, testing, and reviewing all outputs prior to production deployment.",
                    cc = cc
                )

                LegalParagraph(
                    title = "3. Compliance with Third-Party Acceptable Use Policies",
                    content = "Users warrant that their utilization of Dialex adheres to all applicable acceptable use policies and terms established by connected model providers (including the Anthropic Commercial Terms, OpenAI Business Usage Policies, and Google Generative AI Prohibited Use Policy). Dialex must not be deployed to orchestrate malicious, fraudulent, hazardous, or unlawful activities.",
                    cc = cc
                )

                LegalParagraph(
                    title = "4. Intellectual Property & Customer Work Product",
                    content = "Dialex and Kolta Labs assert zero ownership, claim, or intellectual property rights over the prompts you submit, the files you attach, or the deliverables, transcripts, and code synthesized during deliberations. All generated deliverables remain your exclusive work product, subject only to your direct bilateral contracts with third-party model providers.",
                    cc = cc
                )

                LegalParagraph(
                    title = "5. Comprehensive Limitation of Liability",
                    content = "To the maximum extent permitted under applicable law, Kolta Labs, the authors, maintainers, and contributors of Dialex shall not be liable for any direct, indirect, special, incidental, punitive, or consequential damages (including loss of business profits, data corruption, system downtime, production incidents, or security breaches) arising from or relating to your use of or reliance upon Dialex.",
                    cc = cc
                )
            }
        }
    }
}
// Reusable UI Components
@Composable
private fun SectionHeader(title: String, subtitle: String) {
    val cc = LocalCcColors.current
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(
            title,
            style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.SemiBold, fontSize = 15.sp),
            color = cc.textPrimary
        )
        Text(
            subtitle,
            style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp),
            color = cc.textMuted
        )
    }
}


@Composable
private fun OperationalPillarRow(
    pillarNumber: String,
    icon: ImageVector,
    title: String,
    tagline: String,
    description: String,
    highlights: List<String>,
    cc: com.dialex.theme.CcPalette
) {
    Surface(
        shape = RoundedCornerShape(10.dp),
        color = cc.panelAlt,
        border = BorderStroke(0.85.dp, cc.border.copy(alpha = 0.45f)),
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(modifier = Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Surface(
                    shape = RoundedCornerShape(6.dp),
                    color = cc.accent.copy(alpha = 0.12f),
                    border = BorderStroke(0.75.dp, cc.accent.copy(alpha = 0.35f))
                ) {
                    Text(
                        pillarNumber,
                        style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.Bold, fontSize = 11.sp),
                        color = cc.accent,
                        modifier = Modifier.padding(horizontal = 7.dp, vertical = 2.5.dp)
                    )
                }

                Column(modifier = Modifier.weight(1f)) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        Icon(icon, contentDescription = null, tint = cc.accent, modifier = Modifier.size(15.dp))
                        Text(
                            title,
                            style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.SemiBold, fontSize = 13.sp),
                            color = cc.textPrimary
                        )
                    }
                    Text(
                        tagline,
                        style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp),
                        color = cc.textMuted
                    )
                }
            }

            Text(
                description,
                style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp, lineHeight = 17.5.sp),
                color = cc.textPrimary.copy(alpha = 0.9f)
            )

            FlowRow(
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                verticalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                highlights.forEach { tag ->
                    Surface(
                        shape = RoundedCornerShape(4.dp),
                        color = cc.panel,
                        border = BorderStroke(0.5.dp, cc.border.copy(alpha = 0.4f))
                    ) {
                        Text(
                            tag,
                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 9.5.sp, fontWeight = FontWeight.Medium),
                            color = cc.textMuted,
                            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp)
                        )
                    }
                }
            }
        }
    }
}


@Composable
private fun ScenarioCard(
    number: String,
    title: String,
    subtitle: String,
    challenge: String,
    councilSetup: String,
    dynamics: String,
    deliverable: String,
    icon: ImageVector,
    cc: com.dialex.theme.CcPalette
) {
    SettingCard {
        Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                Surface(
                    shape = RoundedCornerShape(8.dp),
                    color = cc.accent.copy(alpha = 0.12f),
                    border = BorderStroke(1.dp, cc.accent.copy(alpha = 0.35f))
                ) {
                    Box(modifier = Modifier.padding(horizontal = 10.dp, vertical = 6.dp)) {
                        Text(
                            number,
                            style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.Bold, fontSize = 14.sp),
                            color = cc.accent
                        )
                    }
                }

                Column(modifier = Modifier.weight(1f)) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        Icon(icon, contentDescription = null, tint = cc.accent, modifier = Modifier.size(16.dp))
                        Text(
                            title,
                            style = MaterialTheme.typography.titleSmall.copy(fontWeight = FontWeight.SemiBold, fontSize = 14.sp),
                            color = cc.textPrimary
                        )
                    }
                    Text(
                        subtitle,
                        style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp),
                        color = cc.textMuted
                    )
                }
            }

            HorizontalDivider(color = cc.border.copy(alpha = 0.3f), thickness = 0.75.dp)

            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                ScenarioDetailBlock(
                    label = "Operational Challenge",
                    content = challenge,
                    icon = Icons.Outlined.WarningAmber,
                    labelColor = Color(0xFFF59E0B),
                    cc = cc
                )

                ScenarioDetailBlock(
                    label = "Council Architecture",
                    content = councilSetup,
                    icon = Icons.Outlined.Group,
                    labelColor = cc.accent,
                    cc = cc
                )

                ScenarioDetailBlock(
                    label = "Deliberation Dynamics",
                    content = dynamics,
                    icon = Icons.Outlined.Forum,
                    labelColor = Color(0xFF3B82F6),
                    cc = cc
                )

                ScenarioDetailBlock(
                    label = "Synthesized Deliverable",
                    content = deliverable,
                    icon = Icons.Outlined.AssignmentTurnedIn,
                    labelColor = Color(0xFF10B981),
                    cc = cc
                )
            }
        }
    }
}

@Composable
private fun ScenarioDetailBlock(
    label: String,
    content: String,
    icon: ImageVector,
    labelColor: Color,
    cc: com.dialex.theme.CcPalette
) {
    Surface(
        shape = RoundedCornerShape(6.dp),
        color = cc.panelAlt,
        border = BorderStroke(0.5.dp, cc.border.copy(alpha = 0.35f)),
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(modifier = Modifier.padding(10.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Icon(icon, contentDescription = null, tint = labelColor, modifier = Modifier.size(13.dp))
                Text(
                    label,
                    style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.SemiBold, fontSize = 10.5.sp),
                    color = labelColor
                )
            }
            Text(
                content,
                style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp, lineHeight = 17.sp),
                color = cc.textPrimary
            )
        }
    }
}

@Composable
private fun SpecRow(label: String, value: String, cc: com.dialex.theme.CcPalette) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(label, style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp), color = cc.textMuted)
        Text(value, style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp, fontWeight = FontWeight.Medium), color = cc.textPrimary)
    }
}

@Composable
private fun LegalParagraph(title: String, content: String, cc: com.dialex.theme.CcPalette) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(
            title,
            style = MaterialTheme.typography.bodySmall.copy(fontWeight = FontWeight.SemiBold, fontSize = 12.5.sp),
            color = cc.textPrimary
        )
        Text(
            content,
            style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp, lineHeight = 17.5.sp),
            color = cc.textMuted
        )
    }
}

@Composable
private fun LicensePermissionPill(
    label: String,
    tint: Color,
    cc: com.dialex.theme.CcPalette,
    modifier: Modifier = Modifier
) {
    Surface(
        shape = RoundedCornerShape(5.dp),
        color = tint.copy(alpha = 0.12f),
        border = BorderStroke(0.75.dp, tint.copy(alpha = 0.35f)),
        modifier = modifier
    ) {
        Text(
            label,
            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp, fontWeight = FontWeight.Medium),
            color = tint,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 3.dp)
        )
    }
}

