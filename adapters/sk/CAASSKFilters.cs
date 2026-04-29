// CAASSKFilters.cs
// CAAS v2 — Semantic Kernel filter implementations
// Wires IFunctionInvocationFilter, IAutoFunctionInvocationFilter,
// and IPromptRenderFilter to SKReinforcementService (port 50067).
//
// Usage:
//   var kernel = Kernel.CreateBuilder()
//       .AddOpenAIChatCompletion(...)
//       .Build();
//
//   var caas = new CAASKernelFilters(new CAASKernelConfig
//   {
//       Endpoint       = "localhost:50067",
//       AgentEntityId  = "agent:my-agent-001",
//       Runtime        = SKRuntime.DotNet,
//   });
//
//   await caas.RegisterTaskAsync(kernel, new SKTaskRegistration
//   {
//       DeclaredGoal        = "Analyse Q3 revenue data",
//       PermittedPlugins    = ["SqlPlugin", "FilePlugin"],
//       MaxTurns            = 20,
//       MaxIterations       = 50,
//   });
//
//   kernel.FunctionInvocationFilters.Add(caas.FunctionFilter);
//   kernel.AutoFunctionInvocationFilters.Add(caas.AutoFunctionFilter);
//   kernel.PromptRenderFilters.Add(caas.PromptRenderFilter);

using System;
using System.Collections.Generic;
using System.Linq;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;
using Grpc.Net.Client;
using Microsoft.Extensions.Logging;
using Microsoft.SemanticKernel;

namespace OpenAutonomyx.CAAS.SK;

// ─── Config ──────────────────────────────────────────────────────────────────

public record CAASKernelConfig
{
    public required string Endpoint       { get; init; }
    public required string AgentEntityId  { get; init; }
    public SKRuntime       Runtime        { get; init; } = SKRuntime.DotNet;
    public bool            UseTls         { get; init; } = false;
    // Thresholds — mirrors behavioral-gate defaults
    public int WarnThreshold         { get; init; } = 400;
    public int StrongWarnThreshold   { get; init; } = 600;
    public int BlockThreshold        { get; init; } = 800;
}

public record SKTaskRegistration
{
    public required string         DeclaredGoal         { get; init; }
    public List<string>            PermittedPlugins     { get; init; } = [];
    public List<string>            PermittedFunctions   { get; init; } = [];
    public List<string>            PermittedResources   { get; init; } = [];
    public int                     MaxTurns             { get; init; } = 0;
    public int                     MaxIterations        { get; init; } = 100;
    public string                  ParentTaskId         { get; init; } = "";
}

public enum SKRuntime { DotNet = 1, Python = 2, Java = 3 }

// ─── Exceptions ───────────────────────────────────────────────────────────────

public class CAASFunctionBlockedException(string reason, int driftScore, string reviewToken = "")
    : Exception($"CAAS BLOCK: {reason} (drift={driftScore})")
{
    public string Reason      { get; } = reason;
    public int    DriftScore  { get; } = driftScore;
    public string ReviewToken { get; } = reviewToken;
}

public class CAASPlannerTerminatedException(string reason)
    : Exception($"CAAS PLANNER TERMINATE: {reason}");

public class CAASAgentMessageBlockedException(string reason)
    : Exception($"CAAS A2A BLOCK: {reason}");

// ─── Main filter class ────────────────────────────────────────────────────────

public sealed class CAASKernelFilters : IDisposable
{
    private readonly CAASKernelConfig     _config;
    private readonly ILogger?             _logger;
    private readonly GrpcChannel         _channel;

    // State per task
    private string? _taskId;
    private string? _sessionId;
    private string? _kernelId;
    private int     _turn;
    private int     _iteration;
    private string? _pendingFragment;

    public CAASKernelFilters(CAASKernelConfig config, ILogger? logger = null)
    {
        _config  = config;
        _logger  = logger;
        _channel = config.UseTls
            ? GrpcChannel.ForAddress($"https://{config.Endpoint}")
            : GrpcChannel.ForAddress($"http://{config.Endpoint}");
    }

    // ── Public filter instances ───────────────────────────────────────────────

    public IFunctionInvocationFilter     FunctionFilter     => new FunctionInvocationFilterImpl(this);
    public IAutoFunctionInvocationFilter AutoFunctionFilter => new AutoFunctionInvocationFilterImpl(this);
    public IPromptRenderFilter           PromptRenderFilter => new PromptRenderFilterImpl(this);

    // ── Task lifecycle ────────────────────────────────────────────────────────

    public async Task<string> RegisterTaskAsync(
        Kernel kernel,
        SKTaskRegistration reg,
        string? sessionId = null,
        CancellationToken ct = default)
    {
        _sessionId = sessionId ?? Guid.NewGuid().ToString();
        _kernelId  = kernel.GetHashCode().ToString("x8");
        _turn      = 0;
        _iteration = 0;

        var req = new
        {
            agent_entity_id     = _config.AgentEntityId,
            session_id          = _sessionId,
            kernel_id           = _kernelId,
            declared_goal       = reg.DeclaredGoal,
            permitted_plugins   = reg.PermittedPlugins,
            permitted_functions = reg.PermittedFunctions,
            permitted_resources = reg.PermittedResources,
            max_turns           = reg.MaxTurns,
            max_iterations      = reg.MaxIterations,
            parent_task_id      = reg.ParentTaskId,
            runtime             = (int)_config.Runtime,
        };

        // var stub = new SKReinforcementService.SKReinforcementServiceClient(_channel);
        // var resp = await stub.RegisterTaskAsync(MapToProto(req), cancellationToken: ct);
        var resp = StubRegisterTask(req);

        _taskId = resp.TaskId;
        if (!string.IsNullOrEmpty(resp.InitialConstraintFragment))
            _pendingFragment = resp.InitialConstraintFragment;

        _logger?.LogInformation("CAAS task registered: {TaskId}", _taskId);
        return _taskId;
    }

    public async Task<SKCloseResult> CloseTaskAsync(
        string reason = "COMPLETED",
        string summary = "",
        CancellationToken ct = default)
    {
        if (_taskId is null) return new SKCloseResult();

        var req = new
        {
            task_id    = _taskId,
            session_id = _sessionId,
            kernel_id  = _kernelId,
            reason,
            summary,
        };

        // var stub = new SKReinforcementService.SKReinforcementServiceClient(_channel);
        // var closeResp = await stub.CloseTaskAsync(MapToProto(req), cancellationToken: ct);
        var closeResp = StubCloseTask(req);

        // Purge scoped memory
        var purgeResp = await PurgeTaskMemoryAsync(ct);

        _logger?.LogInformation(
            "CAAS task closed: {TaskId} | drift={Drift} | purged={Purged} keys",
            _taskId, closeResp.FinalDriftScore, purgeResp.KeysPurged);

        _taskId = null;
        return closeResp with { KeysPurged = purgeResp.KeysPurged };
    }

    // ── Memory ────────────────────────────────────────────────────────────────

    public async Task RegisterMemoryKeyAsync(
        string key,
        string collection = "",
        SKMemoryStoreType store = SKMemoryStoreType.InMemory,
        bool highSensitivity = false,
        CancellationToken ct = default)
    {
        var req = new
        {
            task_id     = _taskId,
            session_id  = _sessionId,
            kernel_id   = _kernelId,
            collection,
            key,
            store       = (int)store,
            sensitivity = highSensitivity ? 2 : 1,
        };

        // Fire-and-forget — never blocks SK execution
        _ = Task.Run(async () =>
        {
            try
            {
                // var stub = new SKReinforcementService.SKReinforcementServiceClient(_channel);
                // var resp = await stub.RegisterMemoryKeyAsync(MapToProto(req));
                var resp = StubRegisterMemoryKey(req);
                if (resp.BleedRisk)
                    _logger?.LogWarning("CAAS memory bleed risk: key={Key} origin={Origin}",
                        key, resp.OriginTaskId);
            }
            catch (Exception ex)
            {
                _logger?.LogWarning("CAAS RegisterMemoryKey failed (non-blocking): {Ex}", ex.Message);
            }
        }, ct);

        await Task.CompletedTask;
    }

    public async Task<SKPurgeResult> PurgeTaskMemoryAsync(CancellationToken ct = default)
    {
        var req = new { task_id = _taskId, session_id = _sessionId, kernel_id = _kernelId, dry_run = false };
        // var stub = new SKReinforcementService.SKReinforcementServiceClient(_channel);
        // return MapFromProto(await stub.PurgeTaskMemoryAsync(MapToProto(req), cancellationToken: ct));
        return StubPurgeMemory(req);
    }

    // ── A2A / AgentGroupChat ──────────────────────────────────────────────────

    public async Task CheckAgentMessageAsync(
        string receiverAgentEntityId,
        string channelName,
        string messageContent,
        string messageRole = "assistant",
        bool crossSovereign = false,
        CancellationToken ct = default)
    {
        var req = new
        {
            sender_agent_entity_id   = _config.AgentEntityId,
            receiver_agent_entity_id = receiverAgentEntityId,
            sender_task_id           = _taskId,
            channel_name             = channelName,
            message_content          = messageContent,
            message_role             = messageRole,
            cross_sovereign          = crossSovereign,
            session_id               = _sessionId,
        };

        // var stub = new SKReinforcementService.SKReinforcementServiceClient(_channel);
        // var resp = await stub.CheckAgentMessageAsync(MapToProto(req), cancellationToken: ct);
        var resp = StubAgentMessageCheck(req);

        if (resp.Verdict == "BLOCK")
            throw new CAASAgentMessageBlockedException(resp.Reason);

        if (resp.LateralRiskScore > 0.7f)
            _logger?.LogWarning("CAAS A2A high lateral risk: receiver={Receiver} score={Score:F2}",
                receiverAgentEntityId, resp.LateralRiskScore);
    }

    // ── Internal filter implementations ──────────────────────────────────────

    private async Task<FunctionFilterResult> HandleFunctionPreAsync(
        FunctionInvocationContext ctx,
        CancellationToken ct)
    {
        _turn++;
        var req = new
        {
            agent_entity_id  = _config.AgentEntityId,
            task_id          = _taskId,
            turn_number      = _turn,
            iteration_number = _iteration,
            phase            = 1, // PRE
            function_name    = ctx.Function.Name,
            plugin_name      = ctx.Function.PluginName ?? "",
            arguments_json   = JsonSerializer.Serialize(
                ctx.Arguments.ToDictionary(k => k.Key, k => k.Value?.ToString() ?? "")),
            session_id       = _sessionId,
            runtime          = (int)_config.Runtime,
        };

        // var stub = new SKReinforcementService.SKReinforcementServiceClient(_channel);
        // var resp = await stub.FunctionInvocationFilterAsync(MapToProto(req), cancellationToken: ct);
        var resp = StubFunctionFilter(req);

        _logger?.LogDebug("CAAS FunctionFilter PRE: fn={Fn} verdict={V} drift={D}",
            ctx.Function.Name, resp.Verdict, resp.DriftScore);

        if (resp.Verdict == "BLOCK")
        {
            _logger?.LogError("CAAS BLOCK: fn={Fn} reason={R} drift={D}",
                ctx.Function.Name, resp.Reason, resp.DriftScore);
            throw new CAASFunctionBlockedException(resp.Reason, resp.DriftScore, resp.ReviewToken);
        }

        if (resp.Verdict is "WARN" or "STRONG_WARN")
        {
            _logger?.LogWarning("CAAS {V}: fn={Fn} reason={R} drift={D}",
                resp.Verdict, ctx.Function.Name, resp.Reason, resp.DriftScore);
            if (resp.Remediation == "INJECT_NEXT_TURN")
                await FetchAndStashInjectionAsync(ct);
        }

        return resp;
    }

    private async Task HandleFunctionPostAsync(
        FunctionInvocationContext ctx,
        bool wasCancelled,
        long durationMs,
        CancellationToken ct)
    {
        _ = Task.Run(async () =>
        {
            try
            {
                var req = new
                {
                    agent_entity_id  = _config.AgentEntityId,
                    task_id          = _taskId,
                    turn_number      = _turn,
                    iteration_number = _iteration,
                    phase            = 2, // POST
                    function_name    = ctx.Function.Name,
                    plugin_name      = ctx.Function.PluginName ?? "",
                    arguments_json   = "{}",
                    result_json      = ctx.Result?.ToString() ?? "",
                    was_cancelled    = wasCancelled,
                    duration_ms      = durationMs,
                    session_id       = _sessionId,
                    runtime          = (int)_config.Runtime,
                };
                // var stub = new SKReinforcementService.SKReinforcementServiceClient(_channel);
                // await stub.FunctionInvocationFilterAsync(MapToProto(req));
                StubFunctionFilter(req);
            }
            catch (Exception ex)
            {
                _logger?.LogWarning("CAAS FunctionFilter POST failed (non-blocking): {Ex}", ex.Message);
            }
        }, ct);
        await Task.CompletedTask;
    }

    private async Task<AutoFunctionFilterResult> HandleAutoFunctionIterationAsync(
        AutoFunctionInvocationContext ctx,
        CancellationToken ct)
    {
        _iteration++;

        var history = ctx.ChatHistory
            .Select(m => new { role = m.Role.ToString().ToLower(), content = m.Content ?? "" })
            .ToList();

        var req = new
        {
            agent_entity_id    = _config.AgentEntityId,
            task_id            = _taskId,
            turn_number        = _turn,
            iteration_number   = _iteration,
            chat_history       = history,
            requested_function = ctx.Function?.Name ?? "",
            requested_plugin   = ctx.Function?.PluginName ?? "",
            requested_args_json= "{}",
            function_count     = ctx.Kernel.Plugins.SelectMany(p => p).Count(),
            is_final_iteration = false,
            session_id         = _sessionId,
            runtime            = (int)_config.Runtime,
        };

        // var stub = new SKReinforcementService.SKReinforcementServiceClient(_channel);
        // var resp = await stub.AutoFunctionInvocationFilterAsync(MapToProto(req), cancellationToken: ct);
        var resp = StubAutoFunctionFilter(req);

        _logger?.LogDebug(
            "CAAS AutoFunctionFilter iter={I} verdict={V} drift={D} signals={S}",
            _iteration, resp.Verdict, resp.DriftScore,
            string.Join(",", resp.Signals.Select(s => s.Type)));

        if (resp.TerminatePlanner)
        {
            _logger?.LogError("CAAS TERMINATE PLANNER: iter={I} reason={R}", _iteration, resp.Reason);
            throw new CAASPlannerTerminatedException(resp.Reason);
        }

        if (resp.InjectBeforeNext && !string.IsNullOrEmpty(resp.ConstraintFragment))
            _pendingFragment = resp.ConstraintFragment;

        return resp;
    }

    private async Task<PromptRenderResult> HandlePromptRenderAsync(
        PromptRenderContext ctx,
        PromptRenderPhase phase,
        CancellationToken ct)
    {
        var prompt = ctx.RenderedPrompt ?? "";
        var req = new
        {
            agent_entity_id  = _config.AgentEntityId,
            task_id          = _taskId,
            turn_number      = _turn,
            phase            = (int)phase,
            prompt_template  = phase == PromptRenderPhase.Pre ? prompt : "",
            prompt_hash      = Hash8(prompt),
            rendered_prompt  = phase == PromptRenderPhase.Post ? prompt : "",
            rendered_hash    = phase == PromptRenderPhase.Post ? Hash8(prompt) : "",
            session_id       = _sessionId,
        };

        // var stub = new SKReinforcementService.SKReinforcementServiceClient(_channel);
        // var resp = await stub.PromptRenderFilterAsync(MapToProto(req), cancellationToken: ct);
        var resp = StubPromptRenderFilter(req);

        if (resp.InjectionRequired && !string.IsNullOrEmpty(resp.ConstraintFragment))
        {
            _logger?.LogInformation("CAAS constraint injection at prompt render turn={T}", _turn);
            _pendingFragment = resp.ConstraintFragment;
        }

        return resp;
    }

    private async Task FetchAndStashInjectionAsync(CancellationToken ct)
    {
        try
        {
            var req = new
            {
                agent_entity_id   = _config.AgentEntityId,
                task_id           = _taskId,
                turn_number       = _turn,
                system_prompt_hash= "",
            };
            // var stub = new SKReinforcementService.SKReinforcementServiceClient(_channel);
            // var resp = await stub.PromptRenderFilterAsync(MapToProto(req), cancellationToken: ct);
            var resp = StubPromptRenderFilter(req);
            if (resp.InjectionRequired)
                _pendingFragment = resp.ConstraintFragment;
        }
        catch (Exception ex)
        {
            _logger?.LogWarning("CAAS injection fetch failed: {Ex}", ex.Message);
        }
    }

    // ── Filter inner classes ──────────────────────────────────────────────────

    private sealed class FunctionInvocationFilterImpl(CAASKernelFilters parent)
        : IFunctionInvocationFilter
    {
        public async Task OnFunctionInvocationAsync(
            FunctionInvocationContext context,
            Func<FunctionInvocationContext, Task> next)
        {
            var sw = System.Diagnostics.Stopwatch.StartNew();
            bool blocked = false;

            // PRE — synchronous gate
            try
            {
                await parent.HandleFunctionPreAsync(context, CancellationToken.None);
            }
            catch (CAASFunctionBlockedException)
            {
                // Abort — do NOT call next()
                blocked = true;
                context.Result = new FunctionResult(context.Function, "Blocked by CAAS");
                throw;
            }

            // Inject pending constraints into next prompt if any
            if (parent._pendingFragment is not null)
            {
                // Fragment is picked up by PromptRenderFilter on next prompt build
                // No action needed here — stash is sufficient
            }

            if (!blocked)
            {
                await next(context);
            }

            sw.Stop();

            // POST — async, never blocks
            await parent.HandleFunctionPostAsync(
                context, blocked, sw.ElapsedMilliseconds, CancellationToken.None);
        }
    }

    private sealed class AutoFunctionInvocationFilterImpl(CAASKernelFilters parent)
        : IAutoFunctionInvocationFilter
    {
        // This fires on EACH iteration of the planner's auto-function loop.
        // It gives CAAS v2 CGL visibility into reasoning BEFORE tool selection —
        // the most valuable hook in the SK filter system.
        public async Task OnAutoFunctionInvocationAsync(
            AutoFunctionInvocationContext context,
            Func<AutoFunctionInvocationContext, Task> next)
        {
            // Inspect reasoning BEFORE next() — this is the novel CGL insertion point.
            // At this point, context.ChatHistory contains the LLM's latest reasoning
            // turn including which tool it is planning to select.
            AutoFunctionFilterResult resp;
            try
            {
                resp = await parent.HandleAutoFunctionIterationAsync(
                    context, CancellationToken.None);
            }
            catch (CAASPlannerTerminatedException)
            {
                // Terminate the entire planner loop
                context.Terminate = true;
                return;
            }

            // If CGL says inject, stash fragment (PromptRenderFilter picks it up)
            if (resp.InjectBeforeNext && !string.IsNullOrEmpty(resp.ConstraintFragment))
                parent._pendingFragment = resp.ConstraintFragment;

            await next(context);
        }
    }

    private sealed class PromptRenderFilterImpl(CAASKernelFilters parent)
        : IPromptRenderFilter
    {
        public async Task OnPromptRenderAsync(
            PromptRenderContext context,
            Func<PromptRenderContext, Task> next)
        {
            // PRE render — check for pending injection
            await parent.HandlePromptRenderAsync(context, PromptRenderPhase.Pre, CancellationToken.None);

            // Apply pending fragment BEFORE render
            if (parent._pendingFragment is not null)
            {
                var fragment = parent._pendingFragment;
                parent._pendingFragment = null;
                // Prepend fragment to rendered prompt
                context.RenderedPrompt = string.IsNullOrEmpty(context.RenderedPrompt)
                    ? fragment
                    : $"{fragment}\n\n{context.RenderedPrompt}";
            }

            await next(context);

            // POST render — record final prompt hash
            await parent.HandlePromptRenderAsync(context, PromptRenderPhase.Post, CancellationToken.None);
        }
    }

    // ── Stubs (replace with generated gRPC stubs after `make proto`) ─────────

    private static FunctionFilterResult StubFunctionFilter(object _) =>
        new() { Verdict = "ALLOW", Reason = "", DriftScore = 0, ReviewToken = "", Remediation = "NONE" };

    private static AutoFunctionFilterResult StubAutoFunctionFilter(object _) =>
        new() { Verdict = "CONTINUE", Reason = "", DriftScore = 0, Signals = [],
                InjectBeforeNext = false, ConstraintFragment = "", TerminatePlanner = false, IterationEventId = "" };

    private static PromptRenderResult StubPromptRenderFilter(object _) =>
        new() { Proceed = true, InjectionRequired = false, ConstraintFragment = "", FragmentHash = "" };

    private static RegisterTaskResult StubRegisterTask(object _) =>
        new() { TaskId = Guid.NewGuid().ToString("n"), IntentHash = "stub",
                InitialConstraintFragment = "" };

    private static SKCloseResult StubCloseTask(object _) =>
        new() { Closed = true, FinalDriftScore = 0, TotalTurns = 0,
                TotalIterations = 0, BlockedFunctions = 0, InjectionsSent = 0 };

    private static RegisterMemoryKeyResult StubRegisterMemoryKey(object _) =>
        new() { Registered = true, BleedRisk = false, OriginTaskId = "" };

    private static SKPurgeResult StubPurgeMemory(object _) =>
        new() { KeysPurged = 0, PurgedKeys = [], BleedKeysFound = 0 };

    private static AgentMessageCheckResult StubAgentMessageCheck(object _) =>
        new() { Verdict = "ALLOW", Reason = "", RelayToken = "", LateralRiskScore = 0f };

    private static string Hash8(string s) =>
        Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(s)))[..16].ToLower();

    public void Dispose() => _channel.Dispose();
}

// ─── Result records ───────────────────────────────────────────────────────────

public record FunctionFilterResult
{
    public string Verdict      { get; init; } = "ALLOW";
    public string Reason       { get; init; } = "";
    public int    DriftScore   { get; init; }
    public string ReviewToken  { get; init; } = "";
    public string Remediation  { get; init; } = "NONE";
    public List<string> ActiveSignals { get; init; } = [];
}

public record AutoFunctionFilterResult
{
    public string             Verdict             { get; init; } = "CONTINUE";
    public string             Reason              { get; init; } = "";
    public int                DriftScore          { get; init; }
    public List<PlannerSignalRecord> Signals      { get; init; } = [];
    public bool               InjectBeforeNext    { get; init; }
    public string             ConstraintFragment  { get; init; } = "";
    public bool               TerminatePlanner    { get; init; }
    public string             IterationEventId    { get; init; } = "";
}

public record PlannerSignalRecord
{
    public string Type   { get; init; } = "";
    public float  Weight { get; init; }
    public string Detail { get; init; } = "";
}

public record PromptRenderResult
{
    public bool   Proceed              { get; init; } = true;
    public bool   InjectionRequired    { get; init; }
    public string ConstraintFragment   { get; init; } = "";
    public string FragmentHash         { get; init; } = "";
}

public record RegisterTaskResult
{
    public string TaskId                       { get; init; } = "";
    public string IntentHash                   { get; init; } = "";
    public string InitialConstraintFragment    { get; init; } = "";
}

public record SKCloseResult
{
    public bool   Closed             { get; init; }
    public int    FinalDriftScore    { get; init; }
    public int    TotalTurns         { get; init; }
    public int    TotalIterations    { get; init; }
    public int    BlockedFunctions   { get; init; }
    public int    InjectionsSent     { get; init; }
    public int    KeysPurged         { get; init; }
}

public record RegisterMemoryKeyResult
{
    public bool   Registered    { get; init; }
    public bool   BleedRisk     { get; init; }
    public string OriginTaskId  { get; init; } = "";
}

public record SKPurgeResult
{
    public int          KeysPurged      { get; init; }
    public List<string> PurgedKeys      { get; init; } = [];
    public int          BleedKeysFound  { get; init; }
}

public record AgentMessageCheckResult
{
    public string Verdict          { get; init; } = "ALLOW";
    public string Reason           { get; init; } = "";
    public string RelayToken       { get; init; } = "";
    public float  LateralRiskScore { get; init; }
}

// ─── Enums ────────────────────────────────────────────────────────────────────

public enum SKMemoryStoreType { InMemory = 1, Redis = 2, AzureAISearch = 3, Qdrant = 4, Chroma = 5, CosmosDb = 6 }
internal enum PromptRenderPhase { Pre = 1, Post = 2 }
