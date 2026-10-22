package com.dialex.presentation.connect

import androidx.lifecycle.viewModelScope
import com.dialex.presentation.base.MviViewModel
import kolt.utils.state.AsyncState
import kotlinx.coroutines.launch

class ConnectViewModel(
    private val connectToRemote: suspend (url: String, username: String, password: String) -> Unit,
    private val connectToLocalEngine: suspend () -> Unit,
) : MviViewModel<ConnectState, ConnectIntent, ConnectEffect>(ConnectState()) {

    override fun onIntent(intent: ConnectIntent) {
        when (intent) {
            is ConnectIntent.UrlChanged -> setState { copy(remoteUrl = intent.url) }
            is ConnectIntent.UsernameChanged -> setState { copy(remoteUsername = intent.username) }
            is ConnectIntent.PasswordChanged -> setState { copy(remotePassword = intent.password) }
            is ConnectIntent.DismissError -> setState { copy(error = null) }
            is ConnectIntent.Reset -> setState { copy(startingLocalEngine = false, connectAsync = AsyncState.Idle, error = null) }
            is ConnectIntent.UseThisDevice -> {
                setState { copy(startingLocalEngine = true, error = null) }
                viewModelScope.launch {
                    try {
                        connectToLocalEngine()
                        setState { copy(startingLocalEngine = false) }
                        sendEffect(ConnectEffect.NavigateToMain)
                    } catch (e: Exception) {
                        setState { copy(error = e.message ?: "Failed to start local engine", startingLocalEngine = false) }
                    }
                }
            }
            is ConnectIntent.ConnectToRemote -> {
                val currentState = state.value
                if (currentState.remoteUrl.isBlank() || currentState.remoteUsername.isBlank() || currentState.remotePassword.isBlank()) {
                    setState { copy(error = "All fields are required") }
                    return
                }
                setState { copy(connectAsync = AsyncState.Loading, error = null) }
                viewModelScope.launch {
                    try {
                        connectToRemote(currentState.remoteUrl, currentState.remoteUsername, currentState.remotePassword)
                        setState { copy(connectAsync = AsyncState.Success(Unit)) }
                        sendEffect(ConnectEffect.NavigateToMain)
                    } catch (e: Exception) {
                        setState { copy(error = e.message ?: "Failed to connect to remote engine", connectAsync = AsyncState.Error(e)) }
                    }
                }
            }
        }
    }
}
