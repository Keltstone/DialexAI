package com.dialex.presentation.legal.licensing

import androidx.lifecycle.viewModelScope
import com.dialex.presentation.base.MviViewModel
import kotlinx.collections.immutable.toImmutableList
import kotlinx.collections.immutable.toImmutableSet
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * ViewModel governing the subtle Licensing Dedication & Statutory Notices screen.
 */
class LicensingViewModel : MviViewModel<LicensingState, LicensingIntent, LicensingEffect>(LicensingState()) {

    override fun onIntent(intent: LicensingIntent) {
        when (intent) {
            is LicensingIntent.SelectSection -> {
                setState { copy(selectedSection = intent.section) }
            }

            is LicensingIntent.UpdateSearchQuery -> {
                val query = intent.query
                val filtered = if (query.isBlank()) {
                    state.value.allNotices
                } else {
                    state.value.allNotices.filter { item ->
                        item.name.contains(query, ignoreCase = true) ||
                        item.description.contains(query, ignoreCase = true) ||
                        item.licenseName.contains(query, ignoreCase = true) ||
                        item.copyright.contains(query, ignoreCase = true)
                    }.toImmutableList()
                }
                setState { copy(searchQuery = query, filteredNotices = filtered) }
            }

            is LicensingIntent.ToggleNoticeExpansion -> {
                val currentSet = state.value.expandedNoticeIds.toMutableSet()
                if (currentSet.contains(intent.noticeId)) {
                    currentSet.remove(intent.noticeId)
                } else {
                    currentSet.add(intent.noticeId)
                }
                setState { copy(expandedNoticeIds = currentSet.toImmutableSet()) }
            }

            is LicensingIntent.CopyText -> {
                setState { copy(copiedToast = "${intent.label} copied to clipboard") }
                sendEffect(LicensingEffect.ShowSnackbar("${intent.label} copied to clipboard"))
                scheduleToastDismissal()
            }

            is LicensingIntent.CopyFullLedger -> {
                setState { copy(copiedToast = "Full statutory licensing ledger copied to clipboard") }
                sendEffect(LicensingEffect.ShowSnackbar("Full statutory licensing ledger copied to clipboard"))
                scheduleToastDismissal()
            }

            is LicensingIntent.ClearCopiedToast -> {
                setState { copy(copiedToast = null) }
            }

            is LicensingIntent.BackClicked -> {
                sendEffect(LicensingEffect.NavigateBack)
            }
        }
    }

    private fun scheduleToastDismissal() {
        viewModelScope.launch {
            delay(2500)
            setState { copy(copiedToast = null) }
        }
    }
}
