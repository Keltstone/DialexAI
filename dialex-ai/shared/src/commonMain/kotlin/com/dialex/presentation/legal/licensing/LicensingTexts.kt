package com.dialex.presentation.legal.licensing

import kotlinx.collections.immutable.ImmutableList
import kotlinx.collections.immutable.toImmutableList

/**
 * Authoritative statutory licensing texts, third-party attribution ledger,
 * disclaimers of warranty, and non-deterministic AI safe harbor notices for Dialex AI.
 */
object LicensingTexts {

    const val APPLICATION_NAME = "Dialex AI"
    const val CURRENT_VERSION = "1.0.0"
    const val COPYRIGHT_HOLDER = "Kolta Labs & Dialex AI Contributors"
    const val COPYRIGHT_YEAR = "2026"
    const val PRIMARY_LICENSE_NAME = "PolyForm Noncommercial License 1.0.0"
    const val PRIMARY_LICENSE_URL = "https://polyformproject.org/licenses/noncommercial/1.0.0"
    const val CONTACT_EMAIL = "licensing@koltalabs.com"
    const val PRIVACY_EMAIL = "privacy@koltalabs.com"

    val PrimaryDedicationStatement: String = """
        DIALEX AI — SOFTWARE DEDICATION & STATUTORY LICENSING CHARTER
        Copyright (c) $COPYRIGHT_YEAR $COPYRIGHT_HOLDER.
        Licensed under the $PRIMARY_LICENSE_NAME.
        
        1. NONCOMMERCIAL PERMITTED USE:
        You are permitted to download, compile, execute, inspect, study, test, adapt, and redistribute Dialex AI for any noncommercial purpose—including personal study, academic research, education, hobbyist experimentation, and use by non-profit charitable organizations.
        
        2. COMMERCIAL RESERVATION:
        Any use of the software to run a commercial enterprise, charge fees for analytical services, bundle with proprietary commercial offerings, or support revenue-generating business operations is strictly conditioned upon obtaining a separate commercial enterprise license from Kolta Labs ($CONTACT_EMAIL).
        
        3. SOURCE AVAILABILITY & AUDITABILITY:
        Dialex AI commits to local execution, transparent reproducible builds, verifiable cryptographic provenance, and zero telemetry. You retain 100% ownership and custody of your discussion transcripts, custom personas, workspace configurations, and synthesized deliverables.
    """.trimIndent()

    val StatutoryWarrantyDisclaimer: String = """
        STATUTORY DISCLAIMER OF WARRANTIES (AS-IS PROVISION)
        
        TO THE MAXIMUM EXTENT PERMITTED BY APPLICABLE JURISDICTIONAL LAW:
        THE SOFTWARE IS PROVIDED "AS IS" AND "AS AVAILABLE", WITH ALL FAULTS AND DEFECTS, WITHOUT WARRANTY OF ANY KIND.
        
        THE LICENSOR, AUTHORS, CONTRIBUTORS, AND SPONSORING ENTITIES (INCLUDING KOLTA LABS) EXPRESSLY DISCLAIM ALL WARRANTIES, WHETHER EXPRESS, IMPLIED, STATUTORY, OR OTHERWISE, INCLUDING WITHOUT LIMITATION:
        - ANY IMPLIED WARRANTIES OF MERCHANTABILITY, SATISFACTORY QUALITY, FITNESS FOR A PARTICULAR PURPOSE, OR ACCURACY;
        - ANY WARRANTIES ARISING OUT OF COURSE OF DEALING, USAGE, OR TRADE PRACTICE;
        - ANY WARRANTIES THAT THE SOFTWARE WILL MEET YOUR REQUIREMENTS, ACHIEVE ANY INTENDED RESULTS, BE COMPATIBLE OR WORK WITH ANY OTHER SOFTWARE, APPLICATIONS, SYSTEMS, OR SERVICES; AND
        - ANY WARRANTIES THAT THE OPERATION OF THE SOFTWARE WILL BE UNINTERRUPTED, SECURE, BUG-FREE, OR ERROR-FREE.
    """.trimIndent()

    val StatutoryLimitationOfLiability: String = """
        COMPREHENSIVE LIMITATION OF LIABILITY
        
        TO THE MAXIMUM EXTENT PERMITTED UNDER APPLICABLE LAW, IN NO EVENT SHALL KOLTA LABS, ITS DIRECTORS, EMPLOYEES, AFFILIATES, OR CONTRIBUTORS BE LIABLE FOR:
        
        (A) ANY INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, PUNITIVE, OR CONSEQUENTIAL DAMAGES WHATSOEVER;
        (B) LOSS OF PROFITS, REVENUE, DATA, GOODWILL, REPUTATIONAL CAPITAL, OR BUSINESS OPPORTUNITIES;
        (C) CORRUPTION OF STORAGE MEDIA, SYSTEM CRASHES, PRODUCTION INCIDENTS, WORK STOPPAGES, OR COMPUTER HARDWARE FAILURES; OR
        (D) ANY THIRD-PARTY RELIANCE OR ACTIONS UNDERTAKEN BASED UPON SYNTHETIC MACHINE LEARNING OUTPUTS,
        
        WHETHER BASED IN CONTRACT, TORT (INCLUDING NEGLIGENCE), STRICT LIABILITY, STATUTORY DUTY, OR OTHERWISE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGES.
        
        THE TOTAL AGGREGATE LIABILITY FOR ALL CLAIMS ARISING OUT OF OR RELATING TO THIS SOFTWARE SHALL IN NO EVENT EXCEED $0.00 USD (ZERO UNITED STATES DOLLARS) OR THE ACTUAL SUM PAID BY YOU TO KOLTA LABS FOR USE OF THE SOFTWARE DURING THE PRIOR TWELVE (12) MONTHS.
    """.trimIndent()

    val AiAdvisorySafeHarbor: String = """
        AI DELIBERATION & NON-DETERMINISTIC INFERENCE SAFE HARBOR NOTICE
        
        1. SYNTHETIC NATURE OF MULTI-AGENT INFERENCES:
        Dialex AI orchestrates debates and synthetic deliberations between statistical large language models (such as Claude, GPT, Gemini, Grok, DeepSeek, Mistral, and local Ollama weights). All outputs—including debate messages, transcripts, Architectural Decision Records (ADRs), threat modeling matrices, confidence scores, and code patches—are generated through probabilistic inference.
        
        2. NO CERTIFIED PROFESSIONAL ADVICE:
        The synthesized artifacts and recommendations generated by Dialex AI DO NOT constitute certified software engineering, architectural sign-off, legal counsel, financial audit, medical advice, or safety-critical validation.
        
        3. MANDATORY HUMAN-IN-THE-LOOP VERIFICATION:
        Users maintain affirmative, sole responsibility for reviewing, testing, independently auditing, and validating all synthesized code, architectural configurations, and policy suggestions prior to executing them in any production, staging, or consequential environment.
    """.trimIndent()

    val DataSovereigntyDeclaration: String = """
        DATA SOVEREIGNTY & ZERO-TELEMETRY DECLARATION
        
        1. ZERO DATA HARVESTING:
        Dialex AI contains zero tracking pixels, analytics beacons, behavioral telemetry, error harvest daemons, or remote session recording tools.
        
        2. SOVEREIGN LOCAL RESIDENCY:
        All project records, local JSON storage, encrypted API tokens, discussion branches, and custom personas reside exclusively on your host workstation.
        
        3. DIRECT TRANSPORT PRIVACY:
        When using third-party foundation models via API keys, HTTPS requests travel directly and exclusively between your host device and the designated provider endpoint. Kolta Labs operates no intermediate cloud proxies, intercepting relays, or payload caching brokers.
    """.trimIndent()

    val DefaultThirdPartyNotices: ImmutableList<ThirdPartyNoticeItem> = listOf(
        ThirdPartyNoticeItem(
            id = "kmp-compose",
            name = "Compose Multiplatform & Jetpack Compose",
            copyright = "Copyright (c) 2020-2026 JetBrains s.r.o. and Google LLC",
            licenseName = "Apache License 2.0",
            licenseUrl = "https://www.apache.org/licenses/LICENSE-2.0",
            description = "Declarative multiplatform UI framework for Kotlin targeting Desktop and Android.",
            noticeText = """
                Licensed under the Apache License, Version 2.0 (the "License");
                you may not use this file except in compliance with the License.
                You may obtain a copy of the License at
                
                    http://www.apache.org/licenses/LICENSE-2.0
                
                Unless required by applicable law or agreed to in writing, software
                distributed under the License is distributed on an "AS IS" BASIS,
                WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
                See the License for the specific language governing permissions and
                limitations under the License.
            """.trimIndent()
        ),
        ThirdPartyNoticeItem(
            id = "kotlinx-coroutines",
            name = "Kotlinx Coroutines",
            copyright = "Copyright (c) 2016-2026 JetBrains s.r.o.",
            licenseName = "Apache License 2.0",
            licenseUrl = "https://www.apache.org/licenses/LICENSE-2.0",
            description = "Library support for Kotlin coroutines with multiplatform primitives, Flow, and Channel.",
            noticeText = "Licensed under the Apache License, Version 2.0 (the \"License\")."
        ),
        ThirdPartyNoticeItem(
            id = "kotlinx-serialization",
            name = "Kotlinx Serialization",
            copyright = "Copyright (c) 2017-2026 JetBrains s.r.o.",
            licenseName = "Apache License 2.0",
            licenseUrl = "https://www.apache.org/licenses/LICENSE-2.0",
            description = "Kotlin multiplatform reflectionless JSON and structured serialization framework.",
            noticeText = "Licensed under the Apache License, Version 2.0 (the \"License\")."
        ),
        ThirdPartyNoticeItem(
            id = "kotlinx-collections-immutable",
            name = "Kotlinx Collections Immutable",
            copyright = "Copyright (c) 2016-2026 JetBrains s.r.o.",
            licenseName = "Apache License 2.0",
            licenseUrl = "https://www.apache.org/licenses/LICENSE-2.0",
            description = "Immutable and persistent collection interfaces and implementations for Kotlin.",
            noticeText = "Licensed under the Apache License, Version 2.0 (the \"License\")."
        ),
        ThirdPartyNoticeItem(
            id = "ktor-client",
            name = "Ktor HTTP Client",
            copyright = "Copyright (c) 2014-2026 JetBrains s.r.o.",
            licenseName = "Apache License 2.0",
            licenseUrl = "https://www.apache.org/licenses/LICENSE-2.0",
            description = "Asynchronous multiplatform HTTP client framework with SSE and streaming engine support.",
            noticeText = "Licensed under the Apache License, Version 2.0 (the \"License\")."
        ),
        ThirdPartyNoticeItem(
            id = "touchlab-kermit",
            name = "Kermit Multiplatform Logger",
            copyright = "Copyright (c) 2020 Touchlab",
            licenseName = "Apache License 2.0",
            licenseUrl = "https://www.apache.org/licenses/LICENSE-2.0",
            description = "Lightweight structured logging library for Kotlin Multiplatform projects.",
            noticeText = "Licensed under the Apache License, Version 2.0 (the \"License\")."
        ),
        ThirdPartyNoticeItem(
            id = "square-okio",
            name = "Okio",
            copyright = "Copyright (c) 2013 Square, Inc.",
            licenseName = "Apache License 2.0",
            licenseUrl = "https://www.apache.org/licenses/LICENSE-2.0",
            description = "Modern I/O and byte buffer manipulation library for Kotlin and Java.",
            noticeText = "Licensed under the Apache License, Version 2.0 (the \"License\")."
        ),
        ThirdPartyNoticeItem(
            id = "google-material-icons",
            name = "Google Material Design Icons",
            copyright = "Copyright (c) Google LLC",
            licenseName = "Apache License 2.0",
            licenseUrl = "https://www.apache.org/licenses/LICENSE-2.0",
            description = "Material Design vector icon catalog and glyphs.",
            noticeText = "Licensed under the Apache License, Version 2.0 (the \"License\")."
        ),
        ThirdPartyNoticeItem(
            id = "go-runtime",
            name = "The Go Programming Language (Standard Library & Daemon Runtime)",
            copyright = "Copyright (c) 2009 The Go Authors. All rights reserved.",
            licenseName = "BSD 3-Clause License",
            licenseUrl = "https://golang.org/LICENSE",
            description = "Runtime environment and standard library powering the local Dialex daemon core.",
            noticeText = """
                Redistribution and use in source and binary forms, with or without
                modification, are permitted provided that the following conditions are met:
                
                   * Redistributions of source code must retain the above copyright
                notice, this list of conditions and the following disclaimer.
                   * Redistributions in binary form must reproduce the above copyright
                notice, this list of conditions and the following disclaimer in the
                documentation and/or other materials provided with the distribution.
                   * Neither the name of Google Inc. nor the names of its contributors
                may be used to endorse or promote products derived from this software
                without specific prior written permission.
                
                THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
                AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
                IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
                DISCLAIMED.
            """.trimIndent()
        ),
        ThirdPartyNoticeItem(
            id = "sqldelight",
            name = "SQLDelight / SQLite",
            copyright = "Copyright (c) 2016 Square, Inc.",
            licenseName = "Apache License 2.0",
            licenseUrl = "https://www.apache.org/licenses/LICENSE-2.0",
            description = "Type-safe SQL schema compiler and multiplatform SQLite storage driver.",
            noticeText = "Licensed under the Apache License, Version 2.0 (the \"License\")."
        ),
        ThirdPartyNoticeItem(
            id = "androidx-libraries",
            name = "AndroidX Jetpack Core & Lifecycle",
            copyright = "Copyright (c) The Android Open Source Project",
            licenseName = "Apache License 2.0",
            licenseUrl = "https://www.apache.org/licenses/LICENSE-2.0",
            description = "Core architectural components, viewmodels, and lifecycle observers for Android target.",
            noticeText = "Licensed under the Apache License, Version 2.0 (the \"License\")."
        )
    ).toImmutableList()

    /**
     * Synthesizes the complete statutory legal ledger into a single comprehensive text document.
     */
    fun generateFullLegalLedger(): String {
        val sb = StringBuilder()
        sb.appendLine("================================================================================")
        sb.appendLine("                     DIALEX AI STATUTORY LICENSING LEDGER                      ")
        sb.appendLine("================================================================================")
        sb.appendLine("Application: $APPLICATION_NAME (v$CURRENT_VERSION)")
        sb.appendLine("Maintainer: $COPYRIGHT_HOLDER")
        sb.appendLine("Primary License: $PRIMARY_LICENSE_NAME")
        sb.appendLine()
        sb.appendLine("--------------------------------------------------------------------------------")
        sb.appendLine("1. PRIMARY LICENSING DEDICATION")
        sb.appendLine("--------------------------------------------------------------------------------")
        sb.appendLine(PrimaryDedicationStatement)
        sb.appendLine()
        sb.appendLine("--------------------------------------------------------------------------------")
        sb.appendLine("2. STATUTORY DISCLAIMER OF WARRANTIES")
        sb.appendLine("--------------------------------------------------------------------------------")
        sb.appendLine(StatutoryWarrantyDisclaimer)
        sb.appendLine()
        sb.appendLine("--------------------------------------------------------------------------------")
        sb.appendLine("3. STATUTORY LIMITATION OF LIABILITY")
        sb.appendLine("--------------------------------------------------------------------------------")
        sb.appendLine(StatutoryLimitationOfLiability)
        sb.appendLine()
        sb.appendLine("--------------------------------------------------------------------------------")
        sb.appendLine("4. NON-DETERMINISTIC AI DELIBERATION SAFE HARBOR")
        sb.appendLine("--------------------------------------------------------------------------------")
        sb.appendLine(AiAdvisorySafeHarbor)
        sb.appendLine()
        sb.appendLine("--------------------------------------------------------------------------------")
        sb.appendLine("5. DATA SOVEREIGNTY & PRIVACY WARRANTY")
        sb.appendLine("--------------------------------------------------------------------------------")
        sb.appendLine(DataSovereigntyDeclaration)
        sb.appendLine()
        sb.appendLine("--------------------------------------------------------------------------------")
        sb.appendLine("6. THIRD-PARTY OPEN SOURCE COMPONENT NOTICES")
        sb.appendLine("--------------------------------------------------------------------------------")
        DefaultThirdPartyNotices.forEachIndexed { index, notice ->
            sb.appendLine("[${index + 1}] ${notice.name}")
            sb.appendLine("    Copyright: ${notice.copyright}")
            sb.appendLine("    License: ${notice.licenseName} (${notice.licenseUrl})")
            sb.appendLine("    Description: ${notice.description}")
            sb.appendLine("    Notice:")
            notice.noticeText.lines().forEach { line ->
                sb.appendLine("      $line")
            }
            sb.appendLine()
        }
        sb.appendLine("================================================================================")
        sb.appendLine("END OF STATUTORY LICENSING LEDGER")
        sb.appendLine("================================================================================")
        return sb.toString()
    }
}

/**
 * Data item representing an open source or statutory notice for a third-party dependency.
 */
data class ThirdPartyNoticeItem(
    val id: String,
    val name: String,
    val copyright: String,
    val licenseName: String,
    val licenseUrl: String,
    val description: String,
    val noticeText: String
)
