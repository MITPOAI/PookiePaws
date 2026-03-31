(function () {
  "use strict";

  const STORAGE_KEY = "pookiepaws:canvas:v1";
  const THEME_STORAGE_KEY = "pookiepaws:theme:v1";
  const THEMES = Object.freeze(["light", "dark", "soft"]);

  // UI microcopy guidelines:
  // 1. Lead with the current state before naming the problem.
  // 2. Explain what stayed protected so the user knows nothing leaked or ran unexpectedly.
  // 3. Offer the next safest step in plain language.
  // 4. Avoid blame, panic, and jargon-heavy phrasing in loading, approval, and error states.
  const MICROCOPY = Object.freeze({
    loading: {
      console: "Refreshing the console so you can see the latest protected state.",
      workflow: "Preparing the workflow and checking it against the police layer.",
      brain: "Pookie is mapping your request into safe, observable workflow steps.",
      vault: "Saving these settings to the local vault on this workstation.",
      approvals: "Recording your decision and updating the workflow state."
    },
    success: {
      workflow: "Workflow queued. You can follow each step in the audit trail on the right.",
      brain: "Workflow routed. Watch the audit trail as each step starts and finishes.",
      vault: "Settings saved to the local vault on this workstation.",
      approval: "Decision recorded. The workflow can continue from this point.",
      rejection: "Request declined. Nothing new was sent."
    },
    blocked: {
      title: "That action was paused to protect your workspace",
      detail: "Nothing was sent or changed. Pookie can help you take a safer next step instead.",
      workflow: "The police layer paused that request before it could run.",
      fileAccess: "This file request is paused until someone explicitly allows it."
    },
    errors: {
      generic: "Something interrupted that step. Your current workspace state is still intact.",
      console: "The console could not refresh right now. The last known state is still on screen.",
      brainRequired: "Free-text routing needs a configured brain first. Direct tools are still available below.",
      vault: "Those settings could not be saved just yet. Your existing saved values were left unchanged."
    },
    empty: {
      workflows: "Use a template, a direct skill, or a brain request to start the first workflow.",
      approvals: "Anything that could send data outward will pause here first.",
      filePermissions: "Protected file reads and writes will appear here before they continue.",
      audit: "Runtime activity will appear here automatically as work begins and finishes."
    },
    themes: {
      light: "Light theme is active: bright, crisp, and focused.",
      dark: "Dark theme is active: low-glare and steady for longer sessions.",
      soft: "Pookie Soft is active: warm, calm, and easy to scan."
    },
    approvals: {
      detail: "Nothing has been sent yet. This step is paused until you decide how it should proceed."
    },
    chat: {
      connecting: "Opening a local chat session and connecting it to the live control plane.",
      connected: "Live chat is connected. New prompts will stream their steps here.",
      offline: "Live chat is offline, so new prompts will use the direct HTTP fallback instead.",
      empty: "Start with a natural-language request and Pookie will turn it into visible workflow steps.",
      timelineEmpty: "As chat runs, routed steps and runtime events will appear here in order.",
      sendError: "That message could not be delivered right now. Your current session history is still on screen.",
      sessionReady: "Local session ready",
      cleared: "Session view cleared on this screen. The underlying session remains available if you reconnect."
    }
  });
  const NODE_LIBRARY = {
    goal: {
      label: "Goal",
      description: "Set the campaign objective.",
      config: { goal: "" }
    },
    research: {
      label: "Research",
      description: "Investigate competitor positioning or pricing.",
      config: { focus: "competitor pricing" }
    },
    compare: {
      label: "Compare",
      description: "Summarize and compare findings.",
      config: { focus: "compare findings" }
    },
    validate: {
      label: "Validate",
      description: "Check campaign links or compliance inputs.",
      config: {
        url: "https://example.com/?utm_source=meta&utm_medium=paid_social&utm_campaign=launch"
      }
    },
    draft_sms: {
      label: "Draft SMS",
      description: "Prepare an approval-gated SMS draft.",
      config: {
        campaign_name: "April VIP launch",
        message: "VIP early access is live. Tap to claim your spot.",
        recipient: "+61400000000"
      }
    },
    approval: {
      label: "Approval",
      description: "Pause for human review.",
      config: {}
    },
    send: {
      label: "Send",
      description: "Prepare outbound delivery after approval.",
      config: { channel: "sms" }
    }
  };

  const state = {
    view: "workflows",
    console: null,
    vault: null,
    vaultNotice: "",
    canvas: loadCanvas(),
    audit: [],
    selectedNodeId: null,
    linkSourceId: null,
    drag: null,
    activeApprovalId: null,
    eventSource: null,
    theme: "light",
    lastBrainResponse: null,
    chatSession: null,
    chatMessages: [],
    chatSteps: [],
    chatSocket: null,
    chatSocketReady: false,
    chatReconnectTimer: null,
    chatStatus: MICROCOPY.chat.connecting
  };

  const refs = {};

  primeTheme();
  document.addEventListener("DOMContentLoaded", init);

  function init() {
    cacheRefs();
    state.theme = loadTheme();
    applyTheme(state.theme);
    renderThemeSwitcher();
    bindNavigation();
    bindThemeSwitcher();
    bindCanvas();
    bindForms();
    bindModal();
    bindKeyboard();
    refreshConsoleState();
    startChatControlPlane();
    startAuditStream();
  }

  function cacheRefs() {
    refs.navItems = Array.from(document.querySelectorAll(".nav-item"));
    refs.views = Array.from(document.querySelectorAll(".view"));
    refs.themeButtons = Array.from(document.querySelectorAll("[data-theme-switch]"));
    refs.themeStatus = document.getElementById("theme-status");
    refs.runtimeBadge = document.getElementById("runtime-badge");
    refs.runtimeDetail = document.getElementById("runtime-detail");
    refs.brainBadge = document.getElementById("brain-badge");
    refs.brainDetail = document.getElementById("brain-detail");
    refs.providerFlags = document.getElementById("provider-flags");
    refs.summaryStrip = document.getElementById("summary-strip");
    refs.templateStrip = document.getElementById("template-strip");
    refs.workflowQueue = document.getElementById("workflow-queue");
    refs.approvalSummary = document.getElementById("approval-summary");
    refs.filePermissionSummary = document.getElementById("file-permission-summary");
    refs.approvalsList = document.getElementById("approvals-list");
    refs.filePermissionsList = document.getElementById("file-permissions-list");
    refs.vaultStatusCards = document.getElementById("vault-status-cards");
    refs.auditStream = document.getElementById("audit-stream");
    refs.streamIndicator = document.getElementById("stream-indicator");
    refs.goalForm = document.getElementById("goal-form");
    refs.goalInput = document.getElementById("goal-input");
    refs.chatForm = document.getElementById("chat-form");
    refs.chatInput = document.getElementById("chat-input");
    refs.chatFeed = document.getElementById("chat-feed");
    refs.chatSteps = document.getElementById("chat-steps");
    refs.chatStatus = document.getElementById("chat-status");
    refs.chatConnectionState = document.getElementById("chat-connection-state");
    refs.chatSessionLabel = document.getElementById("chat-session-label");
    refs.chatClear = document.getElementById("chat-clear");
    refs.brainGuard = document.getElementById("brain-guard");
    refs.brainResponse = document.getElementById("brain-response");
    refs.canvasPlanStatus = document.getElementById("canvas-plan-status");
    refs.refreshConsole = document.getElementById("refresh-console");
    refs.openApprovals = document.getElementById("open-approvals");
    refs.runCanvas = document.getElementById("run-canvas");
    refs.resetCanvas = document.getElementById("reset-canvas");
    refs.canvasBoard = document.getElementById("canvas-board");
    refs.canvasLinks = document.getElementById("canvas-links");
    refs.canvasStage = document.getElementById("canvas-stage");
    refs.inspectorContent = document.getElementById("inspector-content");
    refs.vaultForm = document.getElementById("vault-form");
    refs.vaultMessage = document.getElementById("vault-message");
    refs.approvalModal = document.getElementById("approval-modal");
    refs.modalTitle = document.getElementById("modal-title");
    refs.modalDetail = document.getElementById("modal-detail");
    refs.modalDiff = document.getElementById("modal-diff");
    refs.modalApprove = document.getElementById("modal-approve");
    refs.modalReject = document.getElementById("modal-reject");
    refs.modalClose = document.getElementById("modal-close");
  }

  function bindNavigation() {
    refs.navItems.forEach((item) => {
      item.addEventListener("click", () => setView(item.dataset.view));
    });
    refs.refreshConsole.addEventListener("click", () => refreshConsoleState());
    refs.openApprovals.addEventListener("click", () => setView("approvals"));
  }

  function bindThemeSwitcher() {
    refs.themeButtons.forEach((button) => {
      button.addEventListener("click", () => {
        setTheme(button.dataset.themeSwitch);
      });
    });
  }

  function bindCanvas() {
    document.querySelectorAll(".palette-button").forEach((button) => {
      button.addEventListener("click", () => addNode(button.dataset.nodeType));
    });
    refs.runCanvas.addEventListener("click", runCanvasPlan);
    refs.resetCanvas.addEventListener("click", () => {
      state.canvas = defaultCanvas();
      state.selectedNodeId = state.canvas.nodes[0] ? state.canvas.nodes[0].id : null;
      state.linkSourceId = null;
      persistCanvas();
      renderCanvas();
      clearBrainResponse();
      setCanvasMessage("Canvas reset to the starter flow. You can edit it from here.");
    });
  }

  function bindForms() {
    refs.goalForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      const prompt = refs.goalInput.value.trim();
      if (!prompt) {
        setCanvasMessage("Add a campaign goal first so Pookie knows what outcome to plan for.");
        return;
      }
      if (!isBrainEnabled()) {
        showBrainRequired();
        return;
      }
      await dispatchBrain(prompt);
    });

    document.getElementById("utm-form").addEventListener("submit", async (event) => {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      await postWorkflow({
        name: "Validate campaign UTM",
        skill: "utm-validator",
        input: { url: form.get("url") }
      });
    });

    document.getElementById("lead-form").addEventListener("submit", async (event) => {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      await postWorkflow({
        name: "Route CRM lead",
        skill: "salesmanago-lead-router",
        input: {
          email: form.get("email"),
          name: form.get("name"),
          segment: form.get("segment"),
          priority: form.get("priority")
        }
      });
    });

    document.getElementById("sms-form").addEventListener("submit", async (event) => {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      await postWorkflow({
        name: "Draft launch SMS",
        skill: "mitto-sms-drafter",
        input: {
          campaign_name: form.get("campaign_name"),
          message: form.get("message"),
          recipients: [form.get("recipient")],
          test: true
        }
      });
    });

    refs.vaultForm.addEventListener("submit", saveVault);

    refs.chatForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      const prompt = refs.chatInput.value.trim();
      if (!prompt) {
        setChatStatus("Add a request first so Pookie has something concrete to route.");
        return;
      }
      await sendChatPrompt(prompt);
    });
    refs.chatClear.addEventListener("click", () => {
      state.chatMessages = [];
      state.chatSteps = [];
      setChatStatus(MICROCOPY.chat.cleared);
      renderChatPanel();
    });
  }

  function bindModal() {
    refs.modalApprove.addEventListener("click", () => resolveModalApproval("approve"));
    refs.modalReject.addEventListener("click", () => resolveModalApproval("reject"));
    refs.modalClose.addEventListener("click", closeApprovalModal);
    refs.approvalModal.addEventListener("click", (event) => {
      if (event.target === refs.approvalModal) {
        closeApprovalModal();
      }
    });
  }

  function bindKeyboard() {
    document.addEventListener("keydown", (event) => {
      if (event.key !== "Delete" && event.key !== "Backspace") {
        return;
      }
      const target = event.target;
      if (target && /input|textarea|select/i.test(target.tagName)) {
        return;
      }
      if (state.selectedNodeId) {
        removeNode(state.selectedNodeId);
      }
    });
  }

  async function loadAll() {
    const [consoleSnapshot, vaultStatus] = await Promise.all([
      fetchJSON("/api/v1/console"),
      fetchJSON("/api/v1/settings/vault")
    ]);
    state.console = consoleSnapshot;
    state.vault = vaultStatus;
    render();
  }

  function render() {
    renderNavigation();
    renderThemeSwitcher();
    renderSidebarStatus();
    renderSummaryStrip();
    renderTemplates();
    renderWorkflowQueue();
    renderApprovals();
    renderSettings();
    renderCanvas();
    renderAudit();
    renderBrainResponse();
    renderChatPanel();
  }

  function renderNavigation() {
    refs.navItems.forEach((item) => {
      item.classList.toggle("is-active", item.dataset.view === state.view);
    });
    refs.views.forEach((view) => {
      view.classList.toggle("is-active", view.id === `view-${state.view}`);
    });
  }

  function renderSidebarStatus() {
    if (!state.console) {
      return;
    }
    const status = state.console.status;
    const pendingApprovals = Number(status.pending_approvals || 0);
    const pendingFilePermissions = Number(status.pending_file_permissions || 0);
    refs.runtimeBadge.textContent = `${status.workflows} workflows tracked`;
    refs.runtimeDetail.textContent = `${pendingApprovals} approvals and ${pendingFilePermissions} file requests pending in ${status.workspace_root}`;

    const brain = state.console.brain || { enabled: false, provider: "OpenAI-compatible", mode: "disabled" };
    refs.brainBadge.textContent = brain.enabled ? "Enabled" : "Disabled";
    refs.brainDetail.textContent = brain.enabled
      ? `${brain.provider} / ${brain.mode}${brain.model ? ` / ${brain.model}` : ""}`
      : "No provider configured.";

    refs.providerFlags.innerHTML = [
      renderProviderFlag("Brain", state.vault && state.vault.brain && state.vault.brain.configured),
      renderProviderFlag("Salesmanago", state.vault && state.vault.salesmanago && state.vault.salesmanago.configured),
      renderProviderFlag("Mitto", state.vault && state.vault.mitto && state.vault.mitto.configured)
    ].join("");
  }

  function renderSummaryStrip() {
    if (!state.console) {
      return;
    }
    const status = state.console.status;
    const cards = [
      ["Workflow queue", String(status.workflows), "Local runs and structured submissions tracked from one console."],
      ["Pending approvals", String(status.pending_approvals || 0), "Outbound steps stay paused until a person decides."],
      ["File access", String(status.pending_file_permissions || 0), "Workspace reads and writes are operator-visible actions."],
      ["Provider health", providerHealthText(), "Brain, CRM, and SMS readiness across the current vault."],
      ["Event bus", String(status.event_bus.published), "Internal runtime events published since startup."]
    ];
    refs.summaryStrip.innerHTML = cards.map(([label, value, detail]) => `
      <article class="summary-card">
        <span class="status-label">${escapeHTML(label)}</span>
        <strong>${escapeHTML(value)}</strong>
        <p>${escapeHTML(detail)}</p>
      </article>
    `).join("");
  }

  function renderTemplates() {
    const templates = (state.console && state.console.templates) || [];
    refs.templateStrip.innerHTML = templates.map((template, index) => `
      <article class="template-card">
        <div>
          <span class="skill-pill">${escapeHTML(template.skill)}</span>
          <h3>${escapeHTML(template.name)}</h3>
          <p>${escapeHTML(template.description)}</p>
        </div>
        <div class="button-row">
          <button class="button secondary" type="button" data-template-run="${index}">Run now</button>
          <button class="button ghost" type="button" data-template-canvas="${index}">Load to canvas</button>
        </div>
      </article>
    `).join("");

    refs.templateStrip.querySelectorAll("[data-template-run]").forEach((button) => {
      button.addEventListener("click", async () => {
        const template = templates[Number(button.dataset.templateRun)];
        await postWorkflow({
          name: template.name,
          skill: template.skill,
          input: template.input
        });
      });
    });

    refs.templateStrip.querySelectorAll("[data-template-canvas]").forEach((button) => {
      button.addEventListener("click", () => {
        loadTemplateIntoCanvas(templates[Number(button.dataset.templateCanvas)]);
      });
    });
  }

  function renderWorkflowQueue() {
    const workflows = (state.console && state.console.workflows) || [];
    refs.workflowQueue.innerHTML = workflows.length
      ? workflows.slice(0, 8).map((workflow) => `
          <article class="data-card">
            <div>
              <span class="skill-pill">${escapeHTML(workflow.skill)}</span>
              <h3>${escapeHTML(workflow.name)}</h3>
              <p>${workflow.error ? escapeHTML(workflow.error) : "Workflow state stays visible here without opening logs."}</p>
            </div>
            <div class="data-card__meta">
              ${renderStatusPill(workflow.status)}
              <span class="metric-pill">${escapeHTML(new Date(workflow.updated_at).toLocaleString())}</span>
            </div>
          </article>
        `).join("")
      : `<article class="data-card"><h3>No workflows yet</h3><p>${escapeHTML(MICROCOPY.empty.workflows)}</p></article>`;
  }

  function renderApprovals() {
    const approvals = getPendingApprovals();
    const filePermissions = getPendingFilePermissions();
    const approvalMarkup = approvals.length
      ? approvals.map((approval) => renderApprovalCard(approval, true)).join("")
      : `<article class="data-card"><h3>No pending approvals</h3><p>${escapeHTML(MICROCOPY.empty.approvals)}</p></article>`;
    refs.approvalSummary.innerHTML = approvalMarkup;
    refs.approvalsList.innerHTML = approvalMarkup;

    const filePermissionMarkup = filePermissions.length
      ? filePermissions.map((permission) => renderFilePermissionCard(permission)).join("")
      : `<article class="data-card"><h3>No file requests</h3><p>${escapeHTML(MICROCOPY.empty.filePermissions)}</p></article>`;
    refs.filePermissionSummary.innerHTML = filePermissionMarkup;
    refs.filePermissionsList.innerHTML = filePermissionMarkup;

    [refs.approvalSummary, refs.approvalsList].forEach((node) => {
      node.querySelectorAll("[data-approval-open]").forEach((button) => {
        button.addEventListener("click", () => openApprovalModal(button.dataset.approvalOpen));
      });
      node.querySelectorAll("[data-approval-approve]").forEach((button) => {
        button.addEventListener("click", () => resolveApproval(button.dataset.approvalApprove, "approve"));
      });
      node.querySelectorAll("[data-approval-reject]").forEach((button) => {
        button.addEventListener("click", () => resolveApproval(button.dataset.approvalReject, "reject"));
      });
    });

    [refs.filePermissionSummary, refs.filePermissionsList].forEach((node) => {
      node.querySelectorAll("[data-file-approve]").forEach((button) => {
        button.addEventListener("click", () => resolveFilePermission(button.dataset.fileApprove, "approve"));
      });
      node.querySelectorAll("[data-file-reject]").forEach((button) => {
        button.addEventListener("click", () => resolveFilePermission(button.dataset.fileReject, "reject"));
      });
    });
  }

  function renderSettings() {
    if (!state.vault) {
      return;
    }
    refs.vaultStatusCards.innerHTML = [
      vaultStatusCard("Brain", state.vault.brain && state.vault.brain.configured, state.vault.brain && state.vault.brain.mode ? `${state.vault.brain.provider} / ${state.vault.brain.mode}` : "Not configured"),
      vaultStatusCard("Salesmanago", state.vault.salesmanago && state.vault.salesmanago.configured, "CRM lead routing"),
      vaultStatusCard("Mitto", state.vault.mitto && state.vault.mitto.configured, "SMS drafting and send intents")
    ].join("");

    refs.vaultMessage.textContent = state.vaultNotice || (state.vault.can_write
      ? "Vault writes are available because this server is bound to loopback."
      : "To protect secrets, vault writes stay off until this server is bound to loopback.");
    document.getElementById("save-vault").disabled = !state.vault.can_write;
  }

  function renderCanvas() {
    const nodes = state.canvas.nodes || [];
    const edges = state.canvas.edges || [];

    refs.canvasBoard.innerHTML = nodes.map((node) => `
      <article class="canvas-node${node.id === state.selectedNodeId ? " is-selected" : ""}${node.id === state.linkSourceId ? " is-link-source" : ""}" data-node-id="${escapeHTML(node.id)}" style="transform: translate(${node.position.x}px, ${node.position.y}px);">
        <div class="canvas-node__header" data-drag-handle="${escapeHTML(node.id)}">
          <span class="canvas-node__type">${escapeHTML(node.type.replace("_", " "))}</span>
          <div class="canvas-node__title">${escapeHTML(node.label || NODE_LIBRARY[node.type].label)}</div>
        </div>
        <div class="canvas-node__body">${escapeHTML(nodeDescription(node))}</div>
      </article>
    `).join("");

    refs.canvasBoard.querySelectorAll(".canvas-node").forEach((nodeEl) => {
      nodeEl.addEventListener("click", () => {
        const nodeID = nodeEl.dataset.nodeId;
        if (state.linkSourceId && state.linkSourceId !== nodeID) {
          addEdge(state.linkSourceId, nodeID);
          state.linkSourceId = null;
          renderCanvas();
          return;
        }
        state.selectedNodeId = nodeID;
        renderCanvas();
      });
    });

    refs.canvasBoard.querySelectorAll("[data-drag-handle]").forEach((handle) => {
      handle.addEventListener("pointerdown", startDrag);
    });

    refs.canvasLinks.setAttribute("viewBox", "0 0 1240 820");
    refs.canvasLinks.innerHTML = edges.map((edge) => {
      const from = findNode(edge.from);
      const to = findNode(edge.to);
      if (!from || !to) {
        return "";
      }
      const startX = from.position.x + 188;
      const startY = from.position.y + 56;
      const endX = to.position.x;
      const endY = to.position.y + 56;
      const midX = (startX + endX) / 2;
      return `<path d="M ${startX} ${startY} C ${midX} ${startY}, ${midX} ${endY}, ${endX} ${endY}" fill="none" stroke="${escapeAttribute(readThemeVar("--accent", "#2d6cdf"))}" stroke-opacity="0.45" stroke-width="2.5" stroke-linecap="round"></path>`;
    }).join("");

    renderInspector();
  }

  function renderInspector() {
    const node = findNode(state.selectedNodeId);
    if (!node) {
      refs.inspectorContent.innerHTML = `
        <div class="inspector-empty">
          <h3>Select a node</h3>
          <p class="inline-note">Use the palette to add steps, drag them into position, and inspect their settings here.</p>
        </div>
      `;
      return;
    }

    refs.inspectorContent.innerHTML = buildInspectorMarkup(node);

    document.getElementById("inspector-label").addEventListener("input", (event) => {
      updateNode(node.id, { label: event.target.value });
    });

    Array.from(refs.inspectorContent.querySelectorAll("[data-config-key]")).forEach((field) => {
      field.addEventListener("input", (event) => {
        updateNodeConfig(node.id, event.target.dataset.configKey, event.target.value);
      });
    });

    document.getElementById("inspector-link").addEventListener("click", () => {
      state.linkSourceId = state.linkSourceId === node.id ? null : node.id;
      renderCanvas();
    });
    document.getElementById("inspector-delete").addEventListener("click", () => {
      removeNode(node.id);
    });
  }

  function renderAudit() {
    if (!state.audit.length) {
      refs.auditStream.innerHTML = `<article class="audit-entry"><div class="audit-entry__title">Waiting for runtime activity</div><p>${escapeHTML(MICROCOPY.empty.audit)}</p></article>`;
      return;
    }
    refs.auditStream.innerHTML = state.audit.map((entry) => `
      <article class="audit-entry${entry.severity === "error" ? " is-error" : entry.severity === "warning" ? " is-warning" : ""}">
        <div class="audit-entry__meta">
          <span>${escapeHTML(formatTime(entry.timestamp))}</span>
          <span>${escapeHTML(entry.source || "runtime")}</span>
          ${entry.workflow_id ? `<span>${escapeHTML(entry.workflow_id)}</span>` : ""}
        </div>
        <div class="audit-entry__title">${escapeHTML(entry.title || "Runtime activity")}</div>
        <p>${escapeHTML(entry.detail || "")}</p>
      </article>
    `).join("");
  }

  function renderThemeSwitcher() {
    refs.themeButtons.forEach((button) => {
      const active = button.dataset.themeSwitch === state.theme;
      button.classList.toggle("is-active", active);
      button.setAttribute("aria-pressed", String(active));
    });
    if (refs.themeStatus) {
      refs.themeStatus.textContent = MICROCOPY.themes[state.theme] || MICROCOPY.themes.light;
    }
  }

  function renderBrainResponse() {
    if (!refs.brainResponse) {
      return;
    }
    const response = state.lastBrainResponse;
    if (!response) {
      refs.brainResponse.hidden = true;
      refs.brainResponse.className = "response-card";
      refs.brainResponse.innerHTML = "";
      return;
    }

    const classes = ["response-card"];
    if (response.kind === "blocked") {
      classes.push("is-blocked");
    } else if (response.kind === "success") {
      classes.push("is-success");
    } else if (response.kind === "error") {
      classes.push("is-error");
    }

    const meta = [];
    if (response.model) {
      meta.push(`<span class="metric-pill">${escapeHTML(response.model)}</span>`);
    }
    if (response.skill) {
      meta.push(`<span class="metric-pill">${escapeHTML(response.skill)}</span>`);
    }
    if (response.workflowID) {
      meta.push(`<span class="metric-pill">${escapeHTML(response.workflowID)}</span>`);
    }
    if (response.risk) {
      meta.push(`<span class="status-pill">${escapeHTML(response.risk)}</span>`);
    }

    refs.brainResponse.className = classes.join(" ");
    refs.brainResponse.hidden = false;
    refs.brainResponse.innerHTML = `
      <div>
        <p class="eyebrow">${escapeHTML(response.eyebrow || "Console update")}</p>
        <h3>${escapeHTML(response.title)}</h3>
        <p>${escapeHTML(response.detail)}</p>
        ${response.secondary ? `<p>${escapeHTML(response.secondary)}</p>` : ""}
      </div>
      ${meta.length ? `<div class="response-card__meta">${meta.join("")}</div>` : ""}
      ${response.command ? `
        <div class="response-card__command">
          <strong>${escapeHTML(response.command.name || response.command.skill || "Suggested next step")}</strong>
          <code>${escapeHTML(response.command.skill || response.command.action || "workflow")}</code>
          <p>${escapeHTML(commandSummary(response.command))}</p>
        </div>
      ` : ""}
    `;
  }

  function renderChatPanel() {
    if (!refs.chatFeed || !refs.chatSteps) {
      return;
    }

    refs.chatConnectionState.textContent = state.chatSocketReady ? "Live" : "Fallback";
    refs.chatConnectionState.classList.toggle("is-ready", state.chatSocketReady);
    refs.chatConnectionState.classList.toggle("is-warn", !state.chatSocketReady);
    refs.chatSessionLabel.textContent = state.chatSession
      ? `Session ${state.chatSession.id}`
      : "Preparing session";
    refs.chatStatus.textContent = state.chatStatus || MICROCOPY.chat.connecting;

    refs.chatFeed.innerHTML = state.chatMessages.length
      ? state.chatMessages.map((message) => renderChatMessage(message)).join("")
      : `<div class="chat-empty"><p>${escapeHTML(MICROCOPY.chat.empty)}</p></div>`;

    refs.chatSteps.innerHTML = state.chatSteps.length
      ? state.chatSteps.map((step) => renderChatStep(step)).join("")
      : `<div class="chat-empty"><p>${escapeHTML(MICROCOPY.chat.timelineEmpty)}</p></div>`;
  }

  function renderChatMessage(message) {
    const classes = ["chat-message"];
    if (message.role === "user") {
      classes.push("is-user");
    }
    if (message.kind === "blocked") {
      classes.push("is-blocked");
    }
    if (message.kind === "error") {
      classes.push("is-error");
    }
    return `
      <article class="${classes.join(" ")}">
        <div class="chat-message__meta">
          <span class="chat-message__role">${escapeHTML(message.role === "user" ? "You" : "Pookie")}</span>
          <span class="metric-pill">${escapeHTML(formatTime(message.created_at))}</span>
        </div>
        <p>${escapeHTML(message.content)}</p>
        <div class="response-card__meta">
          ${message.skill ? `<span class="metric-pill">${escapeHTML(message.skill)}</span>` : ""}
          ${message.model ? `<span class="metric-pill">${escapeHTML(message.model)}</span>` : ""}
          ${message.workflow_id ? `<span class="metric-pill">${escapeHTML(message.workflow_id)}</span>` : ""}
        </div>
      </article>
    `;
  }

  function renderChatStep(step) {
    return `
      <article class="chat-step${step.severity === "warning" ? " is-warning" : step.severity === "error" ? " is-error" : ""}">
        <div class="chat-step__meta">
          <span class="chat-step__stage">${escapeHTML(step.stage)}</span>
          <span class="metric-pill">${escapeHTML(formatTime(step.timestamp))}</span>
        </div>
        <strong class="chat-step__title">${escapeHTML(step.title)}</strong>
        <p>${escapeHTML(step.detail)}</p>
        ${step.workflow_id ? `<div class="response-card__meta"><span class="metric-pill">${escapeHTML(step.workflow_id)}</span></div>` : ""}
      </article>
    `;
  }

  async function startChatControlPlane() {
    try {
      await ensureChatSession();
      connectChatSocket();
    } catch (error) {
      setChatStatus(humanizeError(error, MICROCOPY.chat.offline));
      renderChatPanel();
    }
  }

  async function ensureChatSession() {
    if (state.chatSession && state.chatSession.id) {
      return state.chatSession;
    }
    const session = await fetchJSON("/api/v1/chat/sessions", { method: "POST" });
    state.chatSession = session;
    setChatStatus(MICROCOPY.chat.sessionReady);
    renderChatPanel();
    return session;
  }

  function connectChatSocket() {
    if (!state.chatSession || !state.chatSession.id) {
      return;
    }
    if (state.chatSocket) {
      state.chatSocket.close();
    }

    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const url = `${protocol}://${window.location.host}/api/v1/chat/ws?session_id=${encodeURIComponent(state.chatSession.id)}`;
    const socket = new WebSocket(url);
    state.chatSocket = socket;
    state.chatSocketReady = false;
    setChatStatus(MICROCOPY.chat.connecting);
    renderChatPanel();

    socket.addEventListener("open", () => {
      state.chatSocketReady = true;
      setChatStatus(MICROCOPY.chat.connected);
      renderChatPanel();
    });

    socket.addEventListener("message", (event) => {
      handleChatSocketMessage(event.data);
    });

    socket.addEventListener("close", () => {
      state.chatSocketReady = false;
      setChatStatus(MICROCOPY.chat.offline);
      renderChatPanel();
      scheduleChatReconnect();
    });

    socket.addEventListener("error", () => {
      state.chatSocketReady = false;
      setChatStatus(MICROCOPY.chat.offline);
      renderChatPanel();
    });
  }

  function scheduleChatReconnect() {
    if (state.chatReconnectTimer) {
      window.clearTimeout(state.chatReconnectTimer);
    }
    state.chatReconnectTimer = window.setTimeout(() => {
      state.chatReconnectTimer = null;
      if (!state.chatSocketReady) {
        connectChatSocket();
      }
    }, 2000);
  }

  function handleChatSocketMessage(raw) {
    let payload;
    try {
      payload = JSON.parse(raw);
    } catch (_error) {
      return;
    }

    switch (payload.type) {
    case "session.ready":
      if (payload.session) {
        state.chatSession = payload.session;
        state.chatMessages = Array.isArray(payload.session.messages) ? payload.session.messages : state.chatMessages;
      }
      setChatStatus(MICROCOPY.chat.connected);
      renderChatPanel();
      break;
    case "chat.result":
      if (payload.result) {
        applyChatDispatch(payload.result);
      }
      break;
    case "audit.event":
      if (payload.audit) {
        pushChatStep({
          id: payload.audit.id || `audit_${Date.now()}`,
          stage: payload.audit.type || "audit",
          title: payload.audit.title || "Runtime event",
          detail: payload.audit.detail || "",
          severity: payload.audit.severity || "info",
          workflow_id: payload.audit.workflow_id || "",
          timestamp: payload.audit.timestamp || new Date().toISOString()
        });
      }
      break;
    case "chat.error":
      setChatStatus(payload.error || MICROCOPY.chat.sendError);
      renderChatPanel();
      break;
    default:
      break;
    }
  }

  async function sendChatPrompt(prompt) {
    await ensureChatSession();
    setChatStatus("Sending your request into the control plane.");
    refs.chatInput.value = "";
    renderChatPanel();

    if (state.chatSocketReady && state.chatSocket && state.chatSocket.readyState === window.WebSocket.OPEN) {
      state.chatSocket.send(JSON.stringify({
        type: "chat.send",
        prompt
      }));
      return;
    }

    try {
      const response = await fetchJSON(`/api/v1/chat/sessions/${encodeURIComponent(state.chatSession.id)}/messages`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt })
      });
      applyChatDispatch(response);
      setChatStatus(MICROCOPY.chat.offline);
    } catch (error) {
      setChatStatus(humanizeError(error, MICROCOPY.chat.sendError));
      renderChatPanel();
    }
  }

  function applyChatDispatch(response) {
    if (!response) {
      return;
    }
    if (response.session) {
      state.chatSession = response.session;
    }
    if (response.user_message) {
      pushChatMessage(response.user_message);
    }
    if (Array.isArray(response.steps)) {
      response.steps.forEach((step) => pushChatStep(step));
    }
    if (response.assistant_message) {
      pushChatMessage(response.assistant_message);
    }
    setChatStatus(state.chatSocketReady ? MICROCOPY.chat.connected : MICROCOPY.chat.offline);
    renderChatPanel();
  }

  function pushChatMessage(message) {
    if (!message || !message.id) {
      return;
    }
    const existing = state.chatMessages.findIndex((item) => item.id === message.id);
    if (existing >= 0) {
      state.chatMessages[existing] = message;
    } else {
      state.chatMessages.push(message);
      state.chatMessages = state.chatMessages.slice(-24);
    }
  }

  function pushChatStep(step) {
    if (!step || !step.id) {
      return;
    }
    const existing = state.chatSteps.findIndex((item) => item.id === step.id);
    if (existing >= 0) {
      state.chatSteps[existing] = step;
    } else {
      state.chatSteps.unshift(step);
      state.chatSteps = state.chatSteps.slice(0, 36);
    }
    renderChatPanel();
  }

  function setChatStatus(message) {
    state.chatStatus = message;
  }

  async function postWorkflow(payload, options) {
    const config = options || {};
    setCanvasMessage(config.loadingMessage || MICROCOPY.loading.workflow);
    try {
      const workflow = await fetchJSON("/api/v1/workflows", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
      });
      clearBrainResponse();
      setCanvasMessage(config.successMessage || MICROCOPY.success.workflow);
      pushAuditEntry({
        type: "client.workflow.submitted",
        title: "Workflow queued",
        detail: config.auditDetail || `${payload.name || payload.skill} is now queued for execution.`,
        severity: "info",
        timestamp: new Date().toISOString(),
        workflow_id: workflow.id || ""
      });
      await refreshConsoleState();
      return workflow;
    } catch (error) {
      handleWorkflowSubmissionError(error, payload);
      return null;
    }
  }

  async function dispatchBrain(prompt) {
    refs.brainGuard.hidden = true;
    setBrainResponse({
      kind: "loading",
      eyebrow: "Planning",
      title: "Translating your request into workflow steps",
      detail: MICROCOPY.loading.brain
    });
    setCanvasMessage(MICROCOPY.loading.brain);
    try {
      const result = await fetchJSON("/api/v1/brain/dispatch", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt })
      });

      if (result.blocked) {
        setBrainResponse(buildBlockedBrainResponse(result));
        setCanvasMessage(MICROCOPY.blocked.workflow);
        pushAuditEntry({
          type: "client.workflow.blocked",
          title: "Workflow paused by the police layer",
          detail: buildBlockedDecisionMessage(result.blocked, MICROCOPY.blocked.detail),
          severity: "warning",
          timestamp: new Date().toISOString()
        });
        await refreshConsoleState();
        return null;
      }

      refs.goalInput.value = "";
      setBrainResponse(buildSuccessBrainResponse(result));
      setCanvasMessage(MICROCOPY.success.brain);
      await refreshConsoleState();
      return result;
    } catch (error) {
      const detail = humanizeError(error, MICROCOPY.errors.generic);
      if (error && error.status === 503) {
        showBrainRequired();
      }
      setBrainResponse({
        kind: "error",
        eyebrow: "Pause point",
        title: "Pookie could not route that request just yet",
        detail
      });
      setCanvasMessage(detail);
      pushAuditEntry({
        type: "client.error",
        title: "Brain dispatch paused",
        detail,
        severity: "error",
        timestamp: new Date().toISOString()
      });
      return null;
    }
  }

  async function runCanvasPlan() {
    try {
      const plan = await fetchJSON("/api/v1/workflows/plan", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          goal: refs.goalInput.value.trim(),
          nodes: state.canvas.nodes,
          edges: state.canvas.edges
        })
      });

      if (plan.mode === "workflow" && plan.workflow) {
        setCanvasMessage(plan.summary);
        await postWorkflow(plan.workflow, {
          successMessage: plan.summary || MICROCOPY.success.workflow,
          auditDetail: plan.summary || "Canvas workflow queued from the current graph."
        });
        return;
      }
      if (!isBrainEnabled()) {
        showBrainRequired();
        setView("settings");
        return;
      }
      setCanvasMessage(plan.summary);
      await dispatchBrain(plan.brain_prompt);
    } catch (error) {
      const detail = humanizeError(error, MICROCOPY.errors.generic);
      setCanvasMessage(detail);
      pushAuditEntry({
        type: "client.error",
        title: "Canvas planning paused",
        detail,
        severity: "error",
        timestamp: new Date().toISOString()
      });
    }
  }

  async function saveVault(event) {
    event.preventDefault();
    const form = new FormData(refs.vaultForm);
    const payload = {};
    for (const [key, value] of form.entries()) {
      const trimmed = String(value).trim();
      if (trimmed) {
        payload[key] = trimmed;
      }
    }
    if (!Object.keys(payload).length) {
      state.vaultNotice = "Add at least one setting before saving.";
      renderSettings();
      return;
    }
    state.vaultNotice = MICROCOPY.loading.vault;
    renderSettings();
    try {
      state.vault = await fetchJSON("/api/v1/settings/vault", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
      });
      refs.vaultForm.reset();
      state.vaultNotice = MICROCOPY.success.vault;
      await refreshConsoleState();
    } catch (error) {
      state.vaultNotice = humanizeError(error, MICROCOPY.errors.vault);
      renderSettings();
    }
  }

  async function resolveApproval(id, action) {
    setCanvasMessage(MICROCOPY.loading.approvals);
    try {
      await fetchJSON(`/api/v1/approvals/${id}/${action}`, { method: "POST" });
      pushAuditEntry({
        type: `client.approval.${action}`,
        title: action === "approve" ? "Approval recorded" : "Request declined",
        detail: action === "approve" ? MICROCOPY.success.approval : MICROCOPY.success.rejection,
        severity: action === "approve" ? "info" : "warning",
        timestamp: new Date().toISOString()
      });
      if (state.activeApprovalId === id) {
        closeApprovalModal();
      }
      setCanvasMessage(action === "approve" ? MICROCOPY.success.approval : MICROCOPY.success.rejection);
      await refreshConsoleState();
    } catch (error) {
      const detail = humanizeError(error, MICROCOPY.errors.generic);
      setCanvasMessage(detail);
      pushAuditEntry({
        type: "client.error",
        title: "Approval update paused",
        detail,
        severity: "error",
        timestamp: new Date().toISOString()
      });
    }
  }

  async function resolveFilePermission(id, action) {
    setCanvasMessage(MICROCOPY.loading.approvals);
    try {
      await fetchJSON(`/api/v1/file-permissions/${id}/${action}`, { method: "POST" });
      pushAuditEntry({
        type: `client.file_permission.${action}`,
        title: action === "approve" ? "File access approved" : "File access declined",
        detail: action === "approve"
          ? "File access was approved. The waiting workflow can continue."
          : "File access was declined. The protected path remains unchanged.",
        severity: action === "approve" ? "info" : "warning",
        timestamp: new Date().toISOString()
      });
      setCanvasMessage(action === "approve"
        ? "File access approved. The waiting workflow can continue."
        : "File access declined. The protected path remains unchanged.");
      await refreshConsoleState();
    } catch (error) {
      const detail = humanizeError(error, MICROCOPY.errors.generic);
      setCanvasMessage(detail);
      pushAuditEntry({
        type: "client.error",
        title: "File access decision paused",
        detail,
        severity: "error",
        timestamp: new Date().toISOString()
      });
    }
  }

  async function resolveModalApproval(action) {
    if (!state.activeApprovalId) {
      return;
    }
    await resolveApproval(state.activeApprovalId, action);
  }

  function openApprovalModal(id) {
    const approval = getPendingApprovals().find((item) => item.id === id);
    if (!approval) {
      return;
    }
    state.activeApprovalId = approval.id;
    refs.modalTitle.textContent = `${approval.adapter} / ${approval.action}`;
    refs.modalDetail.textContent = `Workflow ${approval.workflow_id} is paused for a human check-in. ${MICROCOPY.approvals.detail}`;
    refs.modalDiff.innerHTML = approvalDiffMarkup(approval);
    refs.approvalModal.hidden = false;
    document.body.style.overflow = "hidden";
  }

  function closeApprovalModal() {
    state.activeApprovalId = null;
    refs.approvalModal.hidden = true;
    document.body.style.overflow = "";
  }

  function startAuditStream() {
    if (state.eventSource) {
      state.eventSource.close();
    }
    const source = new EventSource("/api/v1/events");
    state.eventSource = source;

    source.onopen = () => {
      refs.streamIndicator.textContent = "Live";
      refs.streamIndicator.classList.add("is-live");
      pushAuditEntry({
        type: "client.connected",
        title: "Audit stream connected",
        detail: "Live runtime updates are flowing into the audit trail.",
        severity: "info",
        timestamp: new Date().toISOString()
      });
    };

    source.onmessage = async (event) => {
      try {
        const payload = JSON.parse(event.data);
        pushAuditEntry(payload, true);
        if (payload.type === "approval.required" && payload.approval_id) {
          await refreshConsoleState();
          openApprovalModal(payload.approval_id);
          return;
        }
        if (typeof payload.type === "string" && payload.type.indexOf("file.access.") === 0) {
          await refreshConsoleState();
        }
      } catch (error) {
        pushAuditEntry({
          type: "client.error",
          title: "Audit parse failed",
          detail: humanizeError(error, MICROCOPY.errors.console),
          severity: "error",
          timestamp: new Date().toISOString()
        });
      }
    };

    source.onerror = () => {
      refs.streamIndicator.textContent = "Reconnecting";
      refs.streamIndicator.classList.remove("is-live");
    };
  }

  function pushAuditEntry(entry, rerender) {
    state.audit.unshift(entry);
    state.audit = state.audit.slice(0, 80);
    if (rerender !== false) {
      renderAudit();
    }
  }

  function setView(view) {
    state.view = view || "workflows";
    renderNavigation();
  }

  function addNode(type) {
    const library = NODE_LIBRARY[type];
    if (!library) {
      return;
    }
    const node = {
      id: `node_${Date.now()}_${Math.random().toString(16).slice(2, 8)}`,
      type,
      label: library.label,
      config: JSON.parse(JSON.stringify(library.config || {})),
      position: nextNodePosition()
    };
    state.canvas.nodes.push(node);
    const previous = findNode(state.selectedNodeId) || state.canvas.nodes[state.canvas.nodes.length - 2];
    if (previous) {
      addEdge(previous.id, node.id, false);
    }
    state.selectedNodeId = node.id;
    persistCanvas();
    renderCanvas();
  }

  function addEdge(from, to, persist = true) {
    if (!from || !to || from === to) {
      return;
    }
    const exists = state.canvas.edges.some((edge) => edge.from === from && edge.to === to);
    if (!exists) {
      state.canvas.edges.push({ from, to });
      if (persist) {
        persistCanvas();
      }
    }
  }

  function removeNode(id) {
    state.canvas.nodes = state.canvas.nodes.filter((node) => node.id !== id);
    state.canvas.edges = state.canvas.edges.filter((edge) => edge.from !== id && edge.to !== id);
    if (state.selectedNodeId === id) {
      state.selectedNodeId = null;
    }
    if (state.linkSourceId === id) {
      state.linkSourceId = null;
    }
    persistCanvas();
    renderCanvas();
  }

  function updateNode(id, patch) {
    const node = findNode(id);
    if (!node) {
      return;
    }
    Object.assign(node, patch);
    persistCanvas();
    renderCanvas();
  }

  function updateNodeConfig(id, key, value) {
    const node = findNode(id);
    if (!node) {
      return;
    }
    node.config = node.config || {};
    node.config[key] = value;
    persistCanvas();
  }

  function startDrag(event) {
    const nodeID = event.currentTarget.dataset.dragHandle;
    const node = findNode(nodeID);
    if (!node) {
      return;
    }
    const stageRect = refs.canvasStage.getBoundingClientRect();
    state.drag = {
      nodeID,
      offsetX: event.clientX - stageRect.left - node.position.x + refs.canvasStage.scrollLeft,
      offsetY: event.clientY - stageRect.top - node.position.y + refs.canvasStage.scrollTop
    };
    window.addEventListener("pointermove", onDrag);
    window.addEventListener("pointerup", stopDrag);
  }

  function onDrag(event) {
    if (!state.drag) {
      return;
    }
    const node = findNode(state.drag.nodeID);
    if (!node) {
      return;
    }
    const stageRect = refs.canvasStage.getBoundingClientRect();
    node.position.x = clamp(event.clientX - stageRect.left - state.drag.offsetX + refs.canvasStage.scrollLeft, 24, 1040);
    node.position.y = clamp(event.clientY - stageRect.top - state.drag.offsetY + refs.canvasStage.scrollTop, 64, 720);
    renderCanvas();
  }

  function stopDrag() {
    if (!state.drag) {
      return;
    }
    state.drag = null;
    persistCanvas();
    window.removeEventListener("pointermove", onDrag);
    window.removeEventListener("pointerup", stopDrag);
  }

  function persistCanvas() {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state.canvas));
  }

  function loadCanvas() {
    try {
      const raw = window.localStorage.getItem(STORAGE_KEY);
      if (!raw) {
        return defaultCanvas();
      }
      const parsed = JSON.parse(raw);
      if (!parsed || !Array.isArray(parsed.nodes) || !Array.isArray(parsed.edges)) {
        return defaultCanvas();
      }
      return parsed;
    } catch (_error) {
      return defaultCanvas();
    }
  }

  function defaultCanvas() {
    return {
      nodes: [
        {
          id: "node_goal",
          type: "goal",
          label: "Goal",
          config: { goal: "Research competitor pricing online and draft an SMS campaign for our leads." },
          position: { x: 84, y: 124 }
        },
        {
          id: "node_research",
          type: "research",
          label: "Research",
          config: { focus: "competitor pricing" },
          position: { x: 368, y: 164 }
        },
        {
          id: "node_draft",
          type: "draft_sms",
          label: "Draft SMS",
          config: {
            campaign_name: "April VIP launch",
            message: "VIP early access is live. Tap to claim your spot.",
            recipient: "+61400000000"
          },
          position: { x: 652, y: 264 }
        },
        {
          id: "node_approval",
          type: "approval",
          label: "Approval",
          config: {},
          position: { x: 936, y: 264 }
        }
      ],
      edges: [
        { from: "node_goal", to: "node_research" },
        { from: "node_research", to: "node_draft" },
        { from: "node_draft", to: "node_approval" }
      ]
    };
  }

  function loadTemplateIntoCanvas(template) {
    if (!template) {
      return;
    }
    clearBrainResponse();
    if (template.skill === "utm-validator") {
      state.canvas = {
        nodes: [
          {
            id: "node_validate",
            type: "validate",
            label: "Validate",
            config: { url: template.input.url || "" },
            position: { x: 220, y: 210 }
          }
        ],
        edges: []
      };
    } else if (template.skill === "mitto-sms-drafter") {
      state.canvas = {
        nodes: [
          {
            id: "node_draft",
            type: "draft_sms",
            label: "Draft SMS",
            config: {
              campaign_name: template.input.campaign_name || "",
              message: template.input.message || "",
              recipient: Array.isArray(template.input.recipients) ? template.input.recipients[0] : ""
            },
            position: { x: 240, y: 210 }
          },
          {
            id: "node_approval",
            type: "approval",
            label: "Approval",
            config: {},
            position: { x: 564, y: 210 }
          }
        ],
        edges: [{ from: "node_draft", to: "node_approval" }]
      };
    } else {
      state.canvas = defaultCanvas();
    }
    state.selectedNodeId = state.canvas.nodes[0] ? state.canvas.nodes[0].id : null;
    state.linkSourceId = null;
    persistCanvas();
    renderCanvas();
    setCanvasMessage(`Loaded "${template.name}" into the canvas. You can adjust it before running.`);
  }

  function buildInspectorMarkup(node) {
    return `
      <h3>${escapeHTML(node.label || NODE_LIBRARY[node.type].label)}</h3>
      <p class="inline-note">${escapeHTML(NODE_LIBRARY[node.type].description)}</p>
      <label class="field">
        Label
        <input id="inspector-label" type="text" value="${escapeAttribute(node.label || NODE_LIBRARY[node.type].label)}">
      </label>
      ${inspectorFields(node)}
      <div class="inspector-actions">
        <button class="button secondary" id="inspector-link" type="button">${state.linkSourceId === node.id ? "Cancel link" : "Start link"}</button>
        <button class="button ghost" id="inspector-delete" type="button">Delete node</button>
      </div>
    `;
  }

  function inspectorFields(node) {
    if (node.type === "draft_sms") {
      return `
        <label class="field">
          Campaign name
          <input data-config-key="campaign_name" type="text" value="${escapeAttribute(node.config.campaign_name || "")}">
        </label>
        <label class="field">
          Recipient
          <input data-config-key="recipient" type="text" value="${escapeAttribute(node.config.recipient || "")}">
        </label>
        <label class="field">
          Message
          <textarea data-config-key="message">${escapeHTML(node.config.message || "")}</textarea>
        </label>
      `;
    }
    if (node.type === "approval") {
      return `<p class="inline-note">Approval nodes do not need extra configuration. They block outbound execution until a human approves it.</p>`;
    }

    const key = node.type === "goal" ? "goal" : node.type === "research" || node.type === "compare" ? "focus" : node.type === "send" ? "channel" : "url";
    const value = node.config && node.config[key] ? String(node.config[key]) : "";
    if (key === "goal") {
      return `
        <label class="field">
          Primary detail
          <textarea data-config-key="${escapeHTML(key)}">${escapeHTML(value)}</textarea>
        </label>
      `;
    }
    return `
      <label class="field">
        Primary detail
        <input data-config-key="${escapeHTML(key)}" type="text" value="${escapeAttribute(value)}">
      </label>
    `;
  }

  function nodeDescription(node) {
    if (node.type === "draft_sms") {
      return node.config && node.config.campaign_name ? node.config.campaign_name : NODE_LIBRARY[node.type].description;
    }
    if (node.type === "validate") {
      return node.config && node.config.url ? node.config.url : NODE_LIBRARY[node.type].description;
    }
    if (node.type === "goal") {
      return node.config && node.config.goal ? node.config.goal : NODE_LIBRARY[node.type].description;
    }
    if (node.config && node.config.focus) {
      return node.config.focus;
    }
    if (node.config && node.config.channel) {
      return node.config.channel;
    }
    return NODE_LIBRARY[node.type].description;
  }

  function nextNodePosition() {
    const count = state.canvas.nodes.length;
    return {
      x: 96 + (count % 4) * 232,
      y: 128 + Math.floor(count / 4) * 148
    };
  }

  function getPendingApprovals() {
    return ((state.console && state.console.approvals) || []).filter((item) => item.state === "pending");
  }

  function getPendingFilePermissions() {
    return ((state.console && state.console.file_permissions) || []).filter((item) => item.state === "pending");
  }

  function findNode(id) {
    return (state.canvas.nodes || []).find((node) => node.id === id);
  }

  function setCanvasMessage(message) {
    refs.canvasPlanStatus.textContent = message;
  }

  function isBrainEnabled() {
    return Boolean(state.console && state.console.brain && state.console.brain.enabled);
  }

  function showBrainRequired() {
    refs.brainGuard.textContent = MICROCOPY.errors.brainRequired;
    refs.brainGuard.hidden = false;
  }

  function renderApprovalCard(approval, compact) {
    return `
      <article class="data-card">
        <div>
          <span class="skill-pill">${escapeHTML(approval.adapter)}</span>
          <h3>${escapeHTML(approval.action)}</h3>
          <p>Nothing has been sent yet. Workflow ${escapeHTML(approval.workflow_id)} is waiting for an operator decision.</p>
        </div>
        <div>${approvalDiffMarkup(approval)}</div>
        <div class="data-card__meta">
          ${renderStatusPill(approval.state)}
          <span class="metric-pill">${escapeHTML(new Date(approval.updated_at).toLocaleString())}</span>
        </div>
        <div class="button-row">
          ${compact ? `<button class="button secondary" type="button" data-approval-open="${escapeHTML(approval.id)}">Inspect</button>` : ""}
          <button class="button" type="button" data-approval-approve="${escapeHTML(approval.id)}">Approve</button>
          <button class="button danger" type="button" data-approval-reject="${escapeHTML(approval.id)}">Reject</button>
        </div>
      </article>
    `;
  }

  function renderFilePermissionCard(permission) {
    return `
      <article class="data-card">
        <div>
          <span class="skill-pill">${escapeHTML(permission.mode)}</span>
          <h3>${escapeHTML(permission.path)}</h3>
          <p>${escapeHTML(permission.requester)} is requesting workspace ${escapeHTML(permission.mode)} access. Nothing will touch this path until you decide.</p>
        </div>
        <div class="data-card__meta">
          ${renderStatusPill(permission.state)}
          <span class="metric-pill">${escapeHTML(new Date(permission.updated_at).toLocaleString())}</span>
        </div>
        <div class="button-row">
          <button class="button" type="button" data-file-approve="${escapeHTML(permission.id)}">Approve</button>
          <button class="button danger" type="button" data-file-reject="${escapeHTML(permission.id)}">Reject</button>
        </div>
      </article>
    `;
  }

  function approvalDiffMarkup(approval) {
    const payload = approval.payload || {};
    const lines = Object.keys(payload).map((key) => {
      const raw = payload[key];
      const value = Array.isArray(raw) ? raw.join(", ") : String(raw);
      return `<div><strong>${escapeHTML(key)}</strong>: ${escapeHTML(value)}</div>`;
    });
    if (!lines.length) {
      lines.push("<div>External action payload prepared and waiting for approval.</div>");
    }
    return lines.join("");
  }

  function renderProviderFlag(label, configured) {
    return `<span class="provider-flag ${configured ? "is-ready" : "is-warn"}">${escapeHTML(label)} ${configured ? "ready" : "waiting"}</span>`;
  }

  function renderStatusPill(status) {
    const variant = status === "completed" || status === "approved" ? " is-ready" : status === "failed" || status === "rejected" ? " is-warn" : "";
    return `<span class="status-pill${variant}">${escapeHTML(status)}</span>`;
  }

  function vaultStatusCard(title, configured, detail) {
    return `
      <article class="vault-status-card">
        <span class="status-label">${escapeHTML(title)}</span>
        <strong>${configured ? "Configured" : "Missing"}</strong>
        <p>${escapeHTML(detail)}</p>
      </article>
    `;
  }

  function providerHealthText() {
    if (!state.vault) {
      return "waiting";
    }
    const ready = ["brain", "salesmanago", "mitto"].filter((key) => {
      const item = state.vault[key];
      return item && item.configured;
    }).length;
    return `${ready}/3 ready`;
  }

  function primeTheme() {
    state.theme = loadTheme();
    applyTheme(state.theme);
  }

  async function refreshConsoleState() {
    try {
      await loadAll();
    } catch (error) {
      const detail = humanizeError(error, MICROCOPY.errors.console);
      setCanvasMessage(detail);
      pushAuditEntry({
        type: "client.error",
        title: "Console refresh paused",
        detail,
        severity: "error",
        timestamp: new Date().toISOString()
      });
    }
  }

  function loadTheme() {
    const stored = safeLocalStorageGet(THEME_STORAGE_KEY);
    if (isValidTheme(stored)) {
      return stored;
    }
    const fallback = document.body && document.body.dataset && document.body.dataset.defaultTheme
      ? document.body.dataset.defaultTheme
      : "dark";
    return isValidTheme(fallback) ? fallback : "dark";
  }

  function setTheme(theme) {
    if (!isValidTheme(theme)) {
      return;
    }
    state.theme = theme;
    applyTheme(theme);
    safeLocalStorageSet(THEME_STORAGE_KEY, theme);
    renderThemeSwitcher();
    renderCanvas();
  }

  function applyTheme(theme) {
    const resolved = isValidTheme(theme) ? theme : "dark";
    document.documentElement.dataset.theme = resolved;
    document.documentElement.style.colorScheme = resolved === "dark" ? "dark" : "light";
  }

  function isValidTheme(theme) {
    return THEMES.indexOf(String(theme || "")) >= 0;
  }

  function setBrainResponse(response) {
    state.lastBrainResponse = response;
    renderBrainResponse();
  }

  function clearBrainResponse() {
    state.lastBrainResponse = null;
    renderBrainResponse();
  }

  function buildSuccessBrainResponse(result) {
    const workflow = result.workflow || {};
    const command = result.command || {};
    return {
      kind: "success",
      eyebrow: "Workflow routed",
      title: firstNonEmpty(command.name, workflow.name, "Your request is in motion"),
      detail: MICROCOPY.success.brain,
      secondary: command.explanation || "The next steps are queued and visible in the audit trail.",
      model: result.model || "",
      skill: command.skill || workflow.skill || "",
      workflowID: workflow.id || "",
      command
    };
  }

  function buildBlockedBrainResponse(result) {
    const blocked = result.blocked || {};
    const alternative = result.alternative || {};
    return {
      kind: "blocked",
      eyebrow: "Protected pause",
      title: MICROCOPY.blocked.title,
      detail: buildBlockedDecisionMessage(blocked, MICROCOPY.blocked.detail),
      secondary: firstNonEmpty(
        alternative.message,
        "Pookie can help you continue with a narrower, safer workflow."
      ),
      model: result.model || "",
      skill: blocked.skill || (result.command && result.command.skill) || "",
      risk: blocked.risk || "",
      command: alternative.command || null
    };
  }

  function handleWorkflowSubmissionError(error, payload) {
    const decision = error && error.payload ? error.payload.decision : null;
    if (decision) {
      clearBrainResponse();
      const detail = buildBlockedDecisionMessage(decision, MICROCOPY.blocked.workflow);
      setCanvasMessage(detail);
      pushAuditEntry({
        type: "client.workflow.blocked",
        title: "Workflow paused by the police layer",
        detail,
        severity: "warning",
        timestamp: new Date().toISOString()
      });
      return;
    }

    const detail = humanizeError(error, MICROCOPY.errors.generic);
    setCanvasMessage(detail);
    pushAuditEntry({
      type: "client.error",
      title: `Workflow for ${payload.skill || "task"} could not start`,
      detail,
      severity: "error",
      timestamp: new Date().toISOString()
    });
  }

  function buildBlockedDecisionMessage(decision, fallback) {
    const base = firstNonEmpty(decision && decision.reason, decision && decision.violation, fallback);
    const risk = decision && decision.risk ? `${capitalize(decision.risk)} check:` : "Safety check:";
    if (/nothing was sent or changed/i.test(base)) {
      return `${risk} ${base}`;
    }
    return `${risk} ${base} Nothing was sent or changed.`;
  }

  function commandSummary(command) {
    if (!command) {
      return "";
    }
    const keys = command.input && typeof command.input === "object" ? Object.keys(command.input) : [];
    if (command.explanation) {
      return command.explanation;
    }
    if (keys.length) {
      return `Prepared with ${keys.join(", ")} as visible workflow inputs.`;
    }
    return "Structured workflow step ready for review.";
  }

  function humanizeError(error, fallback) {
    const message = error && typeof error.message === "string" ? error.message.trim() : "";
    if (/brain required/i.test(message)) {
      return MICROCOPY.errors.brainRequired;
    }
    if (/vault writes/i.test(message)) {
      return "To protect secrets, saving is only available while the server is bound to loopback.";
    }
    if (message) {
      return message;
    }
    return fallback;
  }

  function firstNonEmpty() {
    for (let index = 0; index < arguments.length; index += 1) {
      const value = arguments[index];
      if (typeof value === "string" && value.trim()) {
        return value.trim();
      }
    }
    return "";
  }

  function capitalize(value) {
    const text = String(value || "");
    if (!text) {
      return "";
    }
    return text.charAt(0).toUpperCase() + text.slice(1);
  }

  function safeLocalStorageGet(key) {
    try {
      return window.localStorage.getItem(key);
    } catch (_error) {
      return "";
    }
  }

  function safeLocalStorageSet(key, value) {
    try {
      window.localStorage.setItem(key, value);
    } catch (_error) {
      return;
    }
  }

  function readThemeVar(name, fallback) {
    try {
      const value = window.getComputedStyle(document.documentElement).getPropertyValue(name).trim();
      return value || fallback;
    } catch (_error) {
      return fallback;
    }
  }

  async function fetchJSON(url, options) {
    const response = await fetch(url, options);
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      const error = new Error(payload.error || `Request failed with status ${response.status}`);
      error.payload = payload;
      error.status = response.status;
      error.statusText = response.statusText;
      throw error;
    }
    return payload;
  }

  function escapeHTML(value) {
    return String(value == null ? "" : value)
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;")
      .replaceAll("'", "&#39;");
  }

  function escapeAttribute(value) {
    return escapeHTML(value).replaceAll("`", "&#96;");
  }

  function formatTime(value) {
    try {
      return new Date(value).toLocaleTimeString();
    } catch (_error) {
      return "";
    }
  }

  function clamp(value, min, max) {
    return Math.min(Math.max(value, min), max);
  }
})();
