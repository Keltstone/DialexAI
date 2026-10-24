package com.dialex.presentation.legal.licensing

import kotlinx.collections.immutable.ImmutableList
import kotlinx.collections.immutable.ImmutableSet
import kotlinx.collections.immutable.persistentSetOf

/**
 * Visual sections for organizing statutory licensing information.
 */
enum class LicensingSection(val title: String, val subtitle: String) {
    PRIMARY("Primary Dedication", "PolyForm Noncommercial License 1.0.0 terms and author rights"),
    THIRD_PARTY("Third-Party Components", "Open-source dependencies, copyright notices, and permissive licenses"),
    WARRANTY("Statutory Disclaimers", "UCC 'AS IS' warranty disclaimers and limitation of liability"),
    AI_SAFE_HARBOR("AI Safe Harbor", "Non-deterministic multi-agent consensus advisory and disclaimer"),
    SOVEREIGNTY("Data Sovereignty", "Zero-telemetry charter and local workstation residence guarantee")
}

/**
 * State representation for the Licensing Dedication & Legal Information screen.
 */
data class LicensingState(
    val selectedSection: LicensingSection = LicensingSection.PRIMARY,
    val searchQuery: String = "",
    val allNotices: ImmutableList<ThirdPartyNoticeItem> = LicensingTexts.DefaultThirdPartyNotices,
    val filteredNotices: ImmutableList<ThirdPartyNoticeItem> = LicensingTexts.DefaultThirdPartyNotices,
    val expandedNoticeIds: ImmutableSet<String> = persistentSetOf(),
    val copiedToast: String? = null
)

/**
 * User intents accepted by the Licensing Dedication viewmodel.
 */
sealed interface LicensingIntent {
    data class SelectSection(val section: LicensingSection) : LicensingIntent
    data class UpdateSearchQuery(val query: String) : LicensingIntent
    data class ToggleNoticeExpansion(val noticeId: String) : LicensingIntent
    data class CopyText(val label: String, val content: String) : LicensingIntent
    data object CopyFullLedger : LicensingIntent
    data object ClearCopiedToast : LicensingIntent
    data object BackClicked : LicensingIntent
}

/**
 * One-shot effects dispatched by the Licensing Dedication viewmodel.
 */
sealed interface LicensingEffect {
    data object NavigateBack : LicensingEffect
    data class ShowSnackbar(val message: String) : LicensingEffect
}
