(function () {
  "use strict";

  const STORAGE_KEY = "pookiepaws:canvas:v1";
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
    canvas: loadCanvas(),
    audit: [],
    selectedNodeId: null,
    linkSourceId: null,
    drag: null,
    activeApprovalId: null,
    eventSource: null
  };

  const refs = {};

  document.addEventListener("DOMContentLoaded", init);

  function init() {
    cacheRefs();
    bindNavigation();
    bindCanvas();
    bindForms();
    bindModal();
    bindKeyboard();
    loadAll().catch((error) => {
      pushAuditEntry({
        type: "client.error",
        title: "Console load failed",
        detail: error.message,
        severity: "error",
        timestamp: new Date().toISOString()
      });
    });
    startAuditStream();
  }

  function cacheRefs() {
    refs.navItems = Array.from(document.querySelectorAll(".nav-item"));
    refs.views = Array.from(document.querySelectorAll(".view"));
    refs.runtimeBadge = document.getElementById("runtime-badge");
    refs.runtimeDetail = document.getElementById("runtime-detail");
    refs.brainBadge = document.getElementById("brain-badge");
    refs.brainDetail = document.getElementById("brain-detail");
    refs.providerFlags = document.getElementById("provider-flags");
    refs.summaryStrip = document.getElementById("summary-strip");
    refs.templateStrip = document.getElementById("template-strip");
    refs.workflowQueue = document.getElementById("workflow-queue");
    refs.approvalSummary = document.getElementById("approval-summary");
    refs.approvalsList = document.getElementById("approvals-list");
    refs.vaultStatusCards = document.getElementById("vault-status-cards");
    refs.auditStream = document.getElementById("audit-stream");
    refs.streamIndicator = document.getElementById("stream-indicator");
    refs.goalForm = document.getElementById("goal-form");
    refs.goalInput = document.getElementById("goal-input");
    refs.brainGuard = document.getElementById("brain-guard");
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
    refs.refreshConsole.addEventListener("click", () => loadAll());
    refs.openApprovals.addEventListener("click", () => setView("approvals"));
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
      setCanvasMessage("Canvas reset to the default starter flow.");
    });
  }

  function bindForms() {
    refs.goalForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      const prompt = refs.goalInput.value.trim();
      if (!prompt) {
        setCanvasMessage("Enter a campaign goal before running the brain.");
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
    renderSidebarStatus();
    renderSummaryStrip();
    renderTemplates();
    renderWorkflowQueue();
    renderApprovals();
    renderSettings();
    renderCanvas();
    renderAudit();
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
    refs.runtimeBadge.textContent = `${status.workflows} workflows`;
    refs.runtimeDetail.textContent = `${status.pending_approvals} approvals waiting in ${status.workspace_root}`;

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
      ["Workflow queue", String(status.workflows), "Recent runs tracked locally."],
      ["Pending approvals", String(status.pending_approvals), "Outbound actions stay frozen until approval."],
      ["Provider health", providerHealthText(), "Local-first brain plus live CRM/SMS connectors."],
      ["Event bus", String(status.event_bus.published), "Published internal events since startup."]
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
              <p>${workflow.error ? escapeHTML(workflow.error) : "Workflow state is visible here without opening logs."}</p>
            </div>
            <div class="data-card__meta">
              ${renderStatusPill(workflow.status)}
              <span class="metric-pill">${escapeHTML(new Date(workflow.updated_at).toLocaleString())}</span>
            </div>
          </article>
        `).join("")
      : `<article class="data-card"><h3>No workflows yet</h3><p>Use a template, direct skill, or brain prompt to create the first one.</p></article>`;
  }

  function renderApprovals() {
    const approvals = getPendingApprovals();
    const markup = approvals.length
      ? approvals.map((approval) => renderApprovalCard(approval, true)).join("")
      : `<article class="data-card"><h3>No pending approvals</h3><p>Approval-gated actions will appear here before any outbound delivery occurs.</p></article>`;
    refs.approvalSummary.innerHTML = markup;
    refs.approvalsList.innerHTML = markup;

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

    refs.vaultMessage.textContent = state.vault.can_write
      ? "Vault writes are allowed because the server is bound to loopback."
      : "Vault writes are disabled because this server is not loopback-bound.";
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

    refs.canvasLinks.setAttribute("viewBox", "0 0 1200 760");
    refs.canvasLinks.innerHTML = edges.map((edge) => {
      const from = findNode(edge.from);
      const to = findNode(edge.to);
      if (!from || !to) {
        return "";
      }
      const startX = from.position.x + 180;
      const startY = from.position.y + 54;
      const endX = to.position.x;
      const endY = to.position.y + 54;
      const midX = (startX + endX) / 2;
      return `<path d="M ${startX} ${startY} C ${midX} ${startY}, ${midX} ${endY}, ${endX} ${endY}" fill="none" stroke="rgba(79,107,87,0.55)" stroke-width="2.5" stroke-linecap="round"></path>`;
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
      refs.auditStream.innerHTML = `<article class="audit-entry"><div class="audit-entry__title">Waiting for runtime activity</div><p>Events will stream here over SSE as workflows execute.</p></article>`;
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

  async function postWorkflow(payload) {
    await fetchJSON("/api/v1/workflows", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    await loadAll();
  }

  async function dispatchBrain(prompt) {
    refs.brainGuard.hidden = true;
    await fetchJSON("/api/v1/brain/dispatch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ prompt })
    });
    refs.goalInput.value = "";
    setCanvasMessage("Brain dispatch submitted. Watch the audit trail for progress.");
    await loadAll();
  }

  async function runCanvasPlan() {
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
      await postWorkflow(plan.workflow);
      return;
    }
    if (!isBrainEnabled()) {
      showBrainRequired();
      setView("settings");
      return;
    }
    setCanvasMessage(plan.summary);
    await dispatchBrain(plan.brain_prompt);
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
      refs.vaultMessage.textContent = "Enter at least one setting before saving.";
      return;
    }
    state.vault = await fetchJSON("/api/v1/settings/vault", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    refs.vaultForm.reset();
    refs.vaultMessage.textContent = "Settings saved to the local vault.";
    await loadAll();
  }

  async function resolveApproval(id, action) {
    await fetchJSON(`/api/v1/approvals/${id}/${action}`, { method: "POST" });
    if (state.activeApprovalId === id) {
      closeApprovalModal();
    }
    await loadAll();
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
    refs.modalDetail.textContent = `Workflow ${approval.workflow_id} is waiting for a human decision before the outbound action can continue.`;
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
        detail: "The operator console is receiving runtime events over SSE.",
        severity: "info",
        timestamp: new Date().toISOString()
      });
    };

    source.onmessage = async (event) => {
      try {
        const payload = JSON.parse(event.data);
        pushAuditEntry(payload, true);
        if (payload.type === "approval.required" && payload.approval_id) {
          await loadAll();
          openApprovalModal(payload.approval_id);
        }
      } catch (error) {
        pushAuditEntry({
          type: "client.error",
          title: "Audit parse failed",
          detail: error.message,
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
    node.position.x = clamp(event.clientX - stageRect.left - state.drag.offsetX + refs.canvasStage.scrollLeft, 24, 980);
    node.position.y = clamp(event.clientY - stageRect.top - state.drag.offsetY + refs.canvasStage.scrollTop, 24, 660);
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
          position: { x: 72, y: 80 }
        },
        {
          id: "node_research",
          type: "research",
          label: "Research",
          config: { focus: "competitor pricing" },
          position: { x: 340, y: 120 }
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
          position: { x: 620, y: 220 }
        },
        {
          id: "node_approval",
          type: "approval",
          label: "Approval",
          config: {},
          position: { x: 900, y: 220 }
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
    if (template.skill === "utm-validator") {
      state.canvas = {
        nodes: [
          {
            id: "node_validate",
            type: "validate",
            label: "Validate",
            config: { url: template.input.url || "" },
            position: { x: 180, y: 160 }
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
            position: { x: 220, y: 170 }
          },
          {
            id: "node_approval",
            type: "approval",
            label: "Approval",
            config: {},
            position: { x: 520, y: 170 }
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
    setCanvasMessage(`Loaded "${template.name}" into the canvas.`);
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
      x: 80 + (count % 4) * 220,
      y: 80 + Math.floor(count / 4) * 140
    };
  }

  function getPendingApprovals() {
    return ((state.console && state.console.approvals) || []).filter((item) => item.state === "pending");
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
    refs.brainGuard.hidden = false;
  }

  function renderApprovalCard(approval, compact) {
    return `
      <article class="data-card">
        <div>
          <span class="skill-pill">${escapeHTML(approval.adapter)}</span>
          <h3>${escapeHTML(approval.action)}</h3>
          <p>Workflow ${escapeHTML(approval.workflow_id)} is waiting for an operator decision.</p>
        </div>
        <div>${approvalDiffMarkup(approval)}</div>
        <div class="button-row">
          ${compact ? `<button class="button secondary" type="button" data-approval-open="${escapeHTML(approval.id)}">Inspect</button>` : ""}
          <button class="button" type="button" data-approval-approve="${escapeHTML(approval.id)}">Approve</button>
          <button class="button danger" type="button" data-approval-reject="${escapeHTML(approval.id)}">Reject</button>
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
    const variant = status === "completed" ? " is-ready" : status === "failed" || status === "rejected" ? " is-warn" : "";
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

  async function fetchJSON(url, options) {
    const response = await fetch(url, options);
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(payload.error || `Request failed with status ${response.status}`);
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
