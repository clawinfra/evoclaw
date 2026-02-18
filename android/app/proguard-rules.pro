# EvoClaw Agent ProGuard rules
-keep class io.clawinfra.evoclaw.** { *; }
# Keep native binary extraction code
-keepclassmembers class io.clawinfra.evoclaw.AgentService {
    private java.io.File extractBinary();
}
