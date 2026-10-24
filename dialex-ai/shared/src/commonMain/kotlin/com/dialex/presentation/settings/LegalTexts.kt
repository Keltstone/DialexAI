package com.dialex.presentation.settings

/**
 * Standardized legal documentation and charter texts for Dialex AI by Kolta Labs.
 */
object LegalTexts {
    const val CURRENT_LEGAL_VERSION = "1.0.0"
    const val EFFECTIVE_DATE = "January 1, 2026"
    const val MAINTAINER = "Kolta Labs"
    const val REPOSITORY_URL = "https://github.com/Keltstone/DialexAI"
    const val LICENSING_EMAIL = "licensing@koltalabs.com"
    const val PRIVACY_EMAIL = "privacy@koltalabs.com"

    val FullTermsOfService: String = """
# DIALEX AI — TERMS OF SERVICE & AI ADVISORY NOTICE
Effective Date: $EFFECTIVE_DATE • Version: $CURRENT_LEGAL_VERSION
Maintained by: $MAINTAINER ($REPOSITORY_URL)
Licensing Contact: $LICENSING_EMAIL

1. ACCEPTANCE OF TERMS
By downloading, installing, compiling, executing, accessing, or using Dialex AI (the "Software"), you agree to be legally bound by these Terms of Service. If you do not agree with all terms and conditions, you must immediately terminate use and remove the software from your devices.

2. NATURE OF SYNTHETIC MULTI-AGENT DELIBERATIONS (NON-DETERMINISTIC AI)
Dialex AI is an analytical coordination framework orchestrating structured debates between statistical machine learning models (e.g. Anthropic Claude, OpenAI GPT/Codex, Google Gemini, xAI Grok, DeepSeek, Mistral, Ollama). All outputs—including debate messages, transcripts, Architectural Decision Records (ADRs), STRIDE Threat Modeling Matrices, post-mortems, executive deliverables, risk assessments, and generated code—represent synthetic, non-deterministic machine inferences.

3. ABSOLUTE DISCLAIMER OF PROFESSIONAL ADVICE
DIALEX AI DOES NOT PROVIDE CERTIFIED SOFTWARE ENGINEERING, LEGAL, FINANCIAL, TAX, MEDICAL, ARCHITECTURAL, OR LICENSED ADVICE. You bear the sole and affirmative responsibility to independently verify, audit, compile, and test all recommendations and code prior to production adoption or business reliance.

4. BRING-YOUR-OWN-KEY (BYOK) & PROVIDER COMPLIANCE
Requests made to cloud foundation models travel directly from your host machine to the verified API endpoints of chosen providers. Your relationship with providers is governed solely by your direct bilateral contracts (e.g., Anthropic Commercial Terms, OpenAI Business Terms, Google Generative AI Prohibited Use Policy). You are solely responsible for all API token billing, rate limits, and compliance. Kolta Labs does not resell tokens or arbitrate provider disputes.

5. INTELLECTUAL PROPERTY & WORK PRODUCT OWNERSHIP
You retain 100% exclusive right, title, and ownership of all prompts, attachments, custom personas, debate transcripts, and synthesized deliverables. Kolta Labs asserts zero intellectual property claims over your inputs or generated deliverables and does not harvest or train AI models on your deliberations.

6. POLYFORM NONCOMMERCIAL LICENSE 1.0.0 & COMMERCIAL USAGE
Dialex AI source code is licensed under the PolyForm Noncommercial License 1.0.0. Personal study, academic research, education, hobby, and non-profit use is free. ANY USE TO RUN A COMMERCIAL BUSINESS, PROVIDE SERVICES FOR A FEE, OR CONDUCT FOR-PROFIT ACTIVITIES REQUIRES AN ENTERPRISE COMMERCIAL LICENSE FROM KOLTA LABS ($LICENSING_EMAIL).

7. COMPLETE DISCLAIMER OF WARRANTIES & AS-IS PROVISION
TO THE MAXIMUM EXTENT PERMITTED BY APPLICABLE LAW, THE SOFTWARE IS PROVIDED "AS IS" AND "AS AVAILABLE", WITH ALL FAULTS AND DEFECTS, WITHOUT WARRANTY OF ANY KIND, EXPRESS, IMPLIED, OR STATUTORY, INCLUDING MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, TITLE, AND NON-INFRINGEMENT.

8. COMPREHENSIVE LIMITATION OF LIABILITY
IN NO EVENT SHALL KOLTA LABS, ITS MAINTAINERS, CONTRIBUTORS, OR AFFILIATES BE LIABLE FOR ANY INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, PUNITIVE, OR CONSEQUENTIAL DAMAGES (INCLUDING LOST PROFITS, LOST SAVINGS, DATA CORRUPTION, SYSTEM DOWNTIME, PRODUCTION OUTAGES, OR SECURITY BREACHES). THE AGGREGATE LIABILITY OF KOLTA LABS FOR ALL CLAIMS SHALL BE CAPPED AT ${'$'}0.00 USD OR THE AMOUNT PAID IN THE LAST 12 MONTHS.

9. INDEMNIFICATION
You agree to defend, indemnify, and hold harmless Kolta Labs, its maintainers, and contributors against any third-party claims, liabilities, losses, costs, or damages arising out of your breach of these Terms, violation of third-party provider terms, or deployment of AI-generated advice or code.

10. EXPORT CONTROLS & CAPACITY
You certify that you are at least 18 years of age (or age of majority in your jurisdiction) and comply with all applicable United States and international export control regulations (EAR, OFAC sanctions).

11. GOVERNING LAW & BINDING ARBITRATION
Governed by Delaware law without regard to conflicts of law. All disputes shall be resolved through individual binding arbitration with full waiver of class actions and jury trials.
""".trimIndent()

    val FullPrivacyPolicy: String = """
# DIALEX AI — PRIVACY POLICY & DATA SOVEREIGNTY CHARTER
Effective Date: $EFFECTIVE_DATE • Version: $CURRENT_LEGAL_VERSION
Maintained by: $MAINTAINER ($REPOSITORY_URL)
Data Protection Contact: $PRIVACY_EMAIL

1. ABSOLUTE ZERO-TELEMETRY WARRANTY
Dialex AI is architected from the ground up as a sovereign, zero-telemetry system. It does not collect, record, harvest, profile, or transmit user analytics, tracking pixels, crash beacons, engagement metrics, or behavioral telemetry to Kolta Labs or any third party.

2. LOCAL-FIRST DATA RESIDENCE
All workspace contexts, custom personas, discussion transcripts, JSON stores, and synthesized deliverables reside strictly on your local host device. You retain sole custody and unencumbered ownership of your data at all times.

3. DIRECT-TO-PROVIDER TRANSPORT
When interacting with cloud foundation models (Anthropic, OpenAI, Google, xAI, DeepSeek, Mistral), network requests travel directly from your local machine to the official TLS/HTTPS endpoints of the provider. Dialex AI does NOT proxy, intercept, or inspect payloads through intermediary cloud servers.

4. COMPLETE OFFLINE & AIR-GAPPED OPERATION
When using local foundation models (Ollama, local CLI runners, llama.cpp), Dialex AI operates 100% offline with zero external network connectivity, fully satisfying defense-grade air-gapped isolation and trade-secret confidentiality mandates.

5. HARDWARE-ANCHORED & CRYPTOGRAPHIC SECRET PROTECTION
API keys and authentication tokens are encrypted locally using native operating system cryptographic primitives (macOS Keychain, Linux Secret Service, Android Hardware Keystore / TEE). Secrets are never written to unencrypted logs.

6. IMMEDIATE & PERMANENT DATA PURGING
Deleting a project, discussion, or credential immediately and permanently removes all corresponding records from your local storage without tombstoning, delayed garbage collection, or shadow backups.

7. DATA CONTROLLER STATUS & GDPR COMPLIANCE
You are the sole Data Controller of your workspace. Because Kolta Labs does not host, process, or receive your data, Kolta Labs does not act as a Data Processor, engaging zero sub-processors.
""".trimIndent()

    val PolyFormSummary: String = """
POLYFORM NONCOMMERCIAL LICENSE 1.0.0
<https://polyformproject.org/licenses/noncommercial/1.0.0>
Copyright (c) 2026 Kolta Labs

TERMS SUMMARY:
1. Permitted Purpose: Noncommercial purposes are freely permitted, including personal study, research, education, hobby projects, and experimentation.
2. Noncommercial Organizations: Use by charitable, educational, public safety, or non-profit institutions is permitted.
3. Commercial Restriction: Any use to earn revenue or run a commercial business, directly or indirectly, or provide services for a fee is NOT permitted without an Enterprise License from Kolta Labs.
4. Notices: You must retain copyright and license notices on all copies.
5. Disclaimer: Software is provided AS IS, without warranty or liability of any kind.
""".trimIndent()
}
