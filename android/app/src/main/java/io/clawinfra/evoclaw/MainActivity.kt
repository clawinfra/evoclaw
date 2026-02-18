package io.clawinfra.evoclaw

import android.content.*
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import java.io.File

class MainActivity : ComponentActivity() {

    private var statusReceiver: BroadcastReceiver? = null
    private val agentStatus = mutableStateOf("unknown")
    private val agentPid = mutableStateOf(-1)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Register status broadcast receiver
        statusReceiver = object : BroadcastReceiver() {
            override fun onReceive(context: Context, intent: Intent) {
                agentStatus.value = intent.getStringExtra(AgentService.EXTRA_STATUS) ?: "unknown"
                agentPid.value = intent.getIntExtra(AgentService.EXTRA_PID, -1)
            }
        }
        registerReceiver(
            statusReceiver,
            IntentFilter(AgentService.BROADCAST_STATUS),
            RECEIVER_NOT_EXPORTED
        )

        setContent {
            MaterialTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    AgentDashboard(
                        status = agentStatus.value,
                        pid = agentPid.value,
                        logFile = File(filesDir, "logs/agent.log"),
                        onStart = { startAgentService() },
                        onStop = { stopAgentService() }
                    )
                }
            }
        }
    }

    override fun onDestroy() {
        super.onDestroy()
        statusReceiver?.let { unregisterReceiver(it) }
    }

    private fun startAgentService() {
        val intent = Intent(this, AgentService::class.java).apply {
            action = AgentService.ACTION_START
        }
        startForegroundService(intent)
    }

    private fun stopAgentService() {
        val intent = Intent(this, AgentService::class.java).apply {
            action = AgentService.ACTION_STOP
        }
        startService(intent)
    }
}

@Composable
fun AgentDashboard(
    status: String,
    pid: Int,
    logFile: File,
    onStart: () -> Unit,
    onStop: () -> Unit
) {
    val isRunning = status == "running"
    val logContent = remember { mutableStateOf("") }

    // Load last 50 lines of log
    LaunchedEffect(Unit) {
        if (logFile.exists()) {
            logContent.value = logFile.readLines().takeLast(50).joinToString("\n")
        }
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        // Header
        Text("🦞 EvoClaw Agent", style = MaterialTheme.typography.headlineMedium)

        // Status card
        Card(modifier = Modifier.fillMaxWidth()) {
            Column(modifier = Modifier.padding(16.dp)) {
                Text("Status", style = MaterialTheme.typography.labelMedium)
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        text = if (isRunning) "🟢" else "🔴",
                        style = MaterialTheme.typography.bodyLarge
                    )
                    Spacer(Modifier.width(8.dp))
                    Text(
                        text = status.replaceFirstChar { it.uppercase() },
                        style = MaterialTheme.typography.bodyLarge
                    )
                    if (pid > 0) {
                        Spacer(Modifier.width(8.dp))
                        Text(
                            text = "(PID $pid)",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                }
            }
        }

        // Controls
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(
                onClick = onStart,
                enabled = !isRunning,
                modifier = Modifier.weight(1f)
            ) { Text("Start Agent") }

            OutlinedButton(
                onClick = onStop,
                enabled = isRunning,
                modifier = Modifier.weight(1f)
            ) { Text("Stop") }
        }

        // Log viewer
        Text("Logs", style = MaterialTheme.typography.labelMedium)
        Card(modifier = Modifier.fillMaxWidth().weight(1f)) {
            Text(
                text = logContent.value.ifEmpty { "No logs yet." },
                modifier = Modifier.padding(8.dp),
                style = MaterialTheme.typography.bodySmall.copy(
                    fontFamily = FontFamily.Monospace
                )
            )
        }

        // Config hint
        Text(
            text = "Config: ${logFile.parentFile?.parent}/agent.toml",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
}
