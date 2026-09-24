function _t(key, opts) {
 return typeof window.t === 'function' ? window.t(key, opts) : key;
}

/** HTML ( / &#x2F; escapeHtml ) */
function _tPlain(key, opts) {
 if (typeof window.t !== 'function') return key;
 const base = opts && typeof opts === 'object' ? opts : {};
 const interp = base.interpolation && typeof base.interpolation === 'object' ? base.interpolation : {};
 return window.t(key, {
 ...base,
 interpolation: { escapeValue: false, ...interp }
 });
}

/** create / API agentMode */
const BATCH_QUEUE_AGENT_MODES = ['single', 'eino_single', 'deep', 'plan_execute', 'supervisor'];

function isBatchQueueAgentMode(mode) {
 return BATCH_QUEUE_AGENT_MODES.indexOf(String(mode || '').toLowerCase()) >= 0;
}

/** agentMode (Chat) */
function batchQueueAgentModeLabel(mode) {
 const m = String(mode || 'single').toLowerCase();
 if (m === 'single') return _t('chat.agentModeReactNative');
 if (m === 'eino_single') return _t('chat.agentModeEinoSingle');
 if (m === 'multi' || m === 'deep') return _t('chat.agentModeDeep');
 if (m === 'plan_execute') return _t('chat.agentModePlanExecuteLabel');
 if (m === 'supervisor') return _t('chat.agentModeSupervisorLabel');
 return _t('chat.agentModeReactNative');
}

/** Cron 「 completed」status( status , UI ) */
function getBatchQueueStatusPresentation(queue) {
 const map = {
 pending: { text: _t('tasks.statusPending'), class: 'batch-queue-status-pending' },
 running: { text: _t('tasks.statusRunning'), class: 'batch-queue-status-running' },
 paused: { text: _t('tasks.statusPaused'), class: 'batch-queue-status-paused' },
 completed: { text: _t('tasks.statusCompleted'), class: 'batch-queue-status-completed' },
 cancelled: { text: _t('tasks.statusCancelled'), class: 'batch-queue-status-cancelled' }
 };
 const base = map[queue.status] || { text: queue.status, class: 'batch-queue-status-unknown' };
 const cronOn = queue.scheduleMode === 'cron' && queue.scheduleEnabled !== false;
 const nextStr = queue.nextRunAt ? new Date(queue.nextRunAt).toLocaleString() : '';
 const empty = { sublabel: null, progressNote: null, callout: null };

 if (cronOn && queue.status === 'completed') {
 return {
 text: _t('tasks.statusCronCycleIdle'),
 class: 'batch-queue-status-cron-cycle',
 sublabel: nextStr ? _tPlain('tasks.cronNextRunLine', { time: nextStr }) : null,
 progressNote: _t('tasks.cronRoundDoneProgressHint'),
 callout: _t('tasks.cronRecurringCallout')
 };
 }
 if (cronOn && queue.status === 'running') {
 return {
 text: _t('tasks.statusCronRunning'),
 class: 'batch-queue-status-running batch-queue-cron-active',
 sublabel: nextStr ? _tPlain('tasks.cronNextRunLine', { time: nextStr }) : null,
 progressNote: _t('tasks.cronRunningProgressHint'),
 callout: null
 };
 }
 if (cronOn && queue.status === 'pending' && nextStr) {
 return {
 ...base,
 ...empty,
 sublabel: _tPlain('tasks.cronPendingScheduled', { time: nextStr }),
 progressNote: _t('tasks.cronPendingProgressNote')
 };
 }
 return { ...base, ...empty };
}

/** 「subtask/」( batch_task_manager.queueAllowsTaskListMutationLocked ) */
function batchQueueAllowsSubtaskMutation(queue) {
 if (!queue) return false;
 if (queue.status === 'running') return false;
 const hasRunningSubtask = Array.isArray(queue.tasks) && queue.tasks.some(t => t && t.status === 'running');
 if (hasRunningSubtask) return false;
 return queue.status === 'pending' || queue.status === 'paused' || queue.status === 'completed' || queue.status === 'cancelled';
}
if (typeof escapeHtml === 'undefined') {
 function escapeHtml(text) {
 if (text == null) return '';
 const div = document.createElement('div');
 div.textContent = text;
 return div.innerHTML;
 }
}
const tasksState = {
 allTasks: [],
 filteredTasks: [],
 selectedTasks: new Set(),
 autoRefresh: true,
 refreshInterval: null,
 durationUpdateInterval: null,
 completedTasksHistory: [], // savecompletedTasks
 showHistory: true // record
};
function loadCompletedTasksHistory() {
 try {
 const saved = localStorage.getItem('tasks-completed-history');
 if (saved) {
 const history = JSON.parse(saved);
 const now = Date.now();
 const oneDayAgo = now - 24 * 60 * 60 * 1000;
 tasksState.completedTasksHistory = history.filter(task => {
 const completedTime = task.completedAt || task.startedAt;
 return completedTime && new Date(completedTime).getTime() > oneDayAgo;
 });
 saveCompletedTasksHistory();
 }
 } catch (error) {
 console.error('loadCompletedTasksfailed:', error);
 tasksState.completedTasksHistory = [];
 }
}
function saveCompletedTasksHistory() {
 try {
 localStorage.setItem('tasks-completed-history', JSON.stringify(tasksState.completedTasksHistory));
 } catch (error) {
 console.error('saveCompletedTasksfailed:', error);
 }
}
function updateCompletedTasksHistory(currentTasks) {
 const currentTaskIds = new Set(currentTasks.map(t => t.conversationId));
 if (tasksState.allTasks.length === 0) {
 return;
 }
 
 const previousTaskIds = new Set(tasksState.allTasks.map(t => t.conversationId));
 const justCompleted = tasksState.allTasks.filter(task => {
 return previousTaskIds.has(task.conversationId) && !currentTaskIds.has(task.conversationId);
 });
 justCompleted.forEach(task => {
 const exists = tasksState.completedTasksHistory.some(t => t.conversationId === task.conversationId);
 if (!exists) {
 const finalStatus = ['completed', 'failed', 'timeout', 'cancelled'].includes(task.status) 
 ? task.status 
 : 'completed';
 
 tasksState.completedTasksHistory.push({
 conversationId: task.conversationId,
 message: task.message || 'Tasks',
 startedAt: task.startedAt,
 status: finalStatus,
 completedAt: new Date().toISOString()
 });
 }
 });
 if (tasksState.completedTasksHistory.length > 50) {
 tasksState.completedTasksHistory = tasksState.completedTasksHistory
 .sort((a, b) => new Date(b.completedAt || b.startedAt) - new Date(a.completedAt || a.startedAt))
 .slice(0, 50);
 }
 
 saveCompletedTasksHistory();
}
async function loadTasks() {
 const listContainer = document.getElementById('tasks-list');
 if (!listContainer) return;
 
 listContainer.innerHTML = '<div class="loading-spinner">' + _t('tasks.loadingTasks') + '</div>';

 try {
 const [activeResponse, completedResponse] = await Promise.allSettled([
 apiFetch('/api/agent-loop/tasks'),
 apiFetch('/api/agent-loop/tasks/completed').catch(() => null) // APIdoes not exist,backnull
 ]);
 if (activeResponse.status === 'rejected' || !activeResponse.value || !activeResponse.value.ok) {
 throw new Error(_t('tasks.loadTaskListFailed'));
 }

 const activeResult = await activeResponse.value.json();
 const activeTasks = activeResult.tasks || [];
 let completedTasks = [];
 if (completedResponse.status === 'fulfilled' && completedResponse.value && completedResponse.value.ok) {
 try {
 const completedResult = await completedResponse.value.json();
 completedTasks = completedResult.tasks || [];
 } catch (e) {
 console.warn('parseCompletedTasksfailed:', e);
 }
 }
 tasksState.allTasks = activeTasks;
 if (completedTasks.length > 0) {
 const backendTaskIds = new Set(completedTasks.map(t => t.conversationId));
 const localHistory = tasksState.completedTasksHistory.filter(t => 
 !backendTaskIds.has(t.conversationId)
 );
 tasksState.completedTasksHistory = [
 ...completedTasks.map(t => ({
 conversationId: t.conversationId,
 message: t.message || 'Tasks',
 startedAt: t.startedAt,
 status: t.status || 'completed',
 completedAt: t.completedAt || new Date().toISOString()
 })),
 ...localHistory
 ];
 if (tasksState.completedTasksHistory.length > 50) {
 tasksState.completedTasksHistory = tasksState.completedTasksHistory
 .sort((a, b) => new Date(b.completedAt || b.startedAt) - new Date(a.completedAt || a.startedAt))
 .slice(0, 50);
 }
 
 saveCompletedTasksHistory();
 } else {
 updateCompletedTasksHistory(activeTasks);
 }
 
 updateTaskStats(activeTasks);
 filterAndSortTasks();
 startDurationUpdates();
 } catch (error) {
 console.error('loadTasksfailed:', error);
 listContainer.innerHTML = `
 <div class="tasks-empty">
 <p>${_t('tasks.loadFailedRetry')}: ${escapeHtml(error.message)}</p>
 <button class="btn-secondary" onclick="loadTasks()">${_t('tasks.retry')}</button>
 </div>
 `;
 }
}
function updateTaskStats(tasks) {
 const stats = {
 running: 0,
 cancelling: 0,
 completed: 0,
 failed: 0,
 timeout: 0,
 cancelled: 0,
 total: tasks.length
 };

 tasks.forEach(task => {
 if (task.status === 'running') {
 stats.running++;
 } else if (task.status === 'cancelling') {
 stats.cancelling++;
 } else if (task.status === 'completed') {
 stats.completed++;
 } else if (task.status === 'failed') {
 stats.failed++;
 } else if (task.status === 'timeout') {
 stats.timeout++;
 } else if (task.status === 'cancelled') {
 stats.cancelled++;
 }
 });

 const statRunning = document.getElementById('stat-running');
 const statCancelling = document.getElementById('stat-cancelling');
 const statCompleted = document.getElementById('stat-completed');
 const statTotal = document.getElementById('stat-total');

 if (statRunning) statRunning.textContent = stats.running;
 if (statCancelling) statCancelling.textContent = stats.cancelling;
 if (statCompleted) statCompleted.textContent = stats.completed;
 if (statTotal) statTotal.textContent = stats.total;
}
function filterTasks() {
 filterAndSortTasks();
}
function sortTasks() {
 filterAndSortTasks();
}
function filterAndSortTasks() {
 const statusFilter = document.getElementById('tasks-status-filter')?.value || 'all';
 const sortBy = document.getElementById('tasks-sort-by')?.value || 'time-desc';
 let allTasks = [...tasksState.allTasks];
 if (tasksState.showHistory) {
 const historyTasks = tasksState.completedTasksHistory
 .filter(ht => !tasksState.allTasks.some(t => t.conversationId === ht.conversationId))
 .map(ht => ({ ...ht, isHistory: true }));
 allTasks = [...allTasks, ...historyTasks];
 }
 let filtered = allTasks;
 if (statusFilter === 'active') {
 filtered = tasksState.allTasks.filter(task => 
 task.status === 'running' || task.status === 'cancelling'
 );
 } else if (statusFilter === 'history') {
 filtered = allTasks.filter(task => task.isHistory);
 } else if (statusFilter !== 'all') {
 filtered = allTasks.filter(task => task.status === statusFilter);
 }
 filtered.sort((a, b) => {
 const aTime = new Date(a.completedAt || a.startedAt);
 const bTime = new Date(b.completedAt || b.startedAt);
 
 switch (sortBy) {
 case 'time-asc':
 return aTime - bTime;
 case 'time-desc':
 return bTime - aTime;
 case 'status':
 return (a.status || '').localeCompare(b.status || '');
 case 'message':
 return (a.message || '').localeCompare(b.message || '');
 default:
 return 0;
 }
 });
 
 tasksState.filteredTasks = filtered;
 renderTasks(filtered);
 updateBatchActions();
}
function toggleShowHistory(show) {
 tasksState.showHistory = show;
 localStorage.setItem('tasks-show-history', show ? 'true' : 'false');
 filterAndSortTasks();
}
function calculateDuration(startedAt) {
 if (!startedAt) return _t('tasks.unknown');
 const start = new Date(startedAt);
 const now = new Date();
 const diff = Math.floor((now - start) / 1000);
 
 if (diff < 60) {
 return diff + _t('tasks.durationSeconds');
 } else if (diff < 3600) {
 const minutes = Math.floor(diff / 60);
 const seconds = diff % 60;
 return minutes + _t('tasks.durationMinutes') + ' ' + seconds + _t('tasks.durationSeconds');
 } else {
 const hours = Math.floor(diff / 3600);
 const minutes = Math.floor((diff % 3600) / 60);
 return hours + _t('tasks.durationHours') + ' ' + minutes + _t('tasks.durationMinutes');
 }
}
function startDurationUpdates() {
 if (tasksState.durationUpdateInterval) {
 clearInterval(tasksState.durationUpdateInterval);
 }
 tasksState.durationUpdateInterval = setInterval(() => {
 updateTaskDurations();
 }, 1000);
}
function updateTaskDurations() {
 const taskItems = document.querySelectorAll('.task-item[data-task-id]');
 taskItems.forEach(item => {
 const startedAt = item.dataset.startedAt;
 const status = item.dataset.status;
 const durationEl = item.querySelector('.task-duration');
 
 if (durationEl && startedAt && (status === 'running' || status === 'cancelling')) {
 durationEl.textContent = calculateDuration(startedAt);
 }
 });
}
function renderTasks(tasks) {
 const listContainer = document.getElementById('tasks-list');
 if (!listContainer) return;

 if (tasks.length === 0) {
 listContainer.innerHTML = `
 <div class="tasks-empty">
 <p>${_t('tasks.noMatchingTasks')}</p>
 ${tasksState.allTasks.length === 0 && tasksState.completedTasksHistory.length > 0 ? 
 '<p style="margin-top: 8px; color: var(--text-muted); font-size: 0.875rem;">' + _t('tasks.historyHint') + '</p>' : ''}
 </div>
 `;
 return;
 }
 const statusMap = {
 'running': { text: _t('tasks.statusRunning'), class: 'task-status-running' },
 'cancelling': { text: _t('tasks.statusCancelling'), class: 'task-status-cancelling' },
 'failed': { text: _t('tasks.statusFailed'), class: 'task-status-failed' },
 'timeout': { text: _t('tasks.statusTimeout'), class: 'task-status-timeout' },
 'cancelled': { text: _t('tasks.statusCancelled'), class: 'task-status-cancelled' },
 'completed': { text: _t('tasks.statusCompleted'), class: 'task-status-completed' }
 };
 const activeTasks = tasks.filter(t => !t.isHistory);
 const historyTasks = tasks.filter(t => t.isHistory);

 let html = '';
 if (activeTasks.length > 0) {
 html += activeTasks.map(task => renderTaskItem(task, statusMap)).join('');
 }
 if (historyTasks.length > 0) {
 html += `<div class="tasks-history-section">
 <div class="tasks-history-header">
 <span class="tasks-history-title">` + _t('tasks.recentCompletedTasks') + `</span>
 <button class="btn-secondary btn-small" onclick="clearTasksHistory()">` + _t('tasks.clearHistory') + `</button>
 </div>
 ${historyTasks.map(task => renderTaskItem(task, statusMap, true)).join('')}
 </div>`;
 }
 
 listContainer.innerHTML = html;
}
function renderTaskItem(task, statusMap, isHistory = false) {
 const startedTime = task.startedAt ? new Date(task.startedAt) : null;
 const completedTime = task.completedAt ? new Date(task.completedAt) : null;
 
 const timeText = startedTime && !isNaN(startedTime.getTime())
 ? startedTime.toLocaleString('zh-CN', { 
 year: 'numeric',
 month: '2-digit',
 day: '2-digit',
 hour: '2-digit',
 minute: '2-digit',
 second: '2-digit'
 })
 : _t('tasks.unknownTime');
 
 const completedText = completedTime && !isNaN(completedTime.getTime())
 ? completedTime.toLocaleString('zh-CN', { 
 year: 'numeric',
 month: '2-digit',
 day: '2-digit',
 hour: '2-digit',
 minute: '2-digit',
 second: '2-digit'
 })
 : '';

 const status = statusMap[task.status] || { text: task.status, class: 'task-status-unknown' };
 const isFinalStatus = ['failed', 'timeout', 'cancelled', 'completed'].includes(task.status);
 const canCancel = !isFinalStatus && task.status !== 'cancelling' && !isHistory;
 const isSelected = tasksState.selectedTasks.has(task.conversationId);
 const duration = (task.status === 'running' || task.status === 'cancelling') 
 ? calculateDuration(task.startedAt) 
 : '';

 return `
 <div class="task-item ${isHistory ? 'task-item-history' : ''}" data-task-id="${task.conversationId}" data-started-at="${task.startedAt}" data-status="${task.status}">
 <div class="task-header">
 <div class="task-info">
 ${canCancel ? `
 <label class="task-checkbox">
 <input type="checkbox" ${isSelected ? 'checked' : ''} 
 onchange="toggleTaskSelection('${task.conversationId}', this.checked)">
 </label>
 ` : '<div class="task-checkbox-placeholder"></div>'}
 <span class="task-status ${status.class}">${status.text}</span>
 ${isHistory ? '<span class="task-history-badge" title="' + _t('tasks.historyBadge') + '">' + (typeof window.pyIcon === 'function' ? window.pyIcon('history', { size: 12 }) : '') + '</span>' : ''}
 <span class="task-message" title="${escapeHtml(task.message || _t('tasks.unnamedTask'))}">${escapeHtml(task.message || _t('tasks.unnamedTask'))}</span>
 </div>
 <div class="task-actions">
 ${duration ? `<span class="task-duration" title="${_t('tasks.duration')}">${typeof window.pyIcon === 'function' ? window.pyIcon('timer', { size: 12 }) : ''}${duration}</span>` : ''}
 <span class="task-time" title="${isHistory && completedText ? _t('tasks.completedAt') : _t('tasks.startedAt')}">
 ${isHistory && completedText ? completedText : timeText}
 </span>
 ${canCancel ? `<button class="btn-secondary btn-small" onclick="cancelTask('${task.conversationId}', this)">` + _t('tasks.cancelTask') + `</button>` : ''}
 ${task.conversationId ? `<button class="btn-secondary btn-small" onclick="viewConversation('${task.conversationId}')">` + _t('tasks.viewConversation') + `</button>` : ''}
 </div>
 </div>
 ${task.conversationId ? `
 <div class="task-details">
 <span class="task-id-label">` + _t('tasks.conversationIdLabel') + `:</span>
 <span class="task-id-value" title="` + _t('tasks.clickToCopy') + `" onclick="copyTaskId('${task.conversationId}')">${escapeHtml(task.conversationId)}</span>
 </div>
 ` : ''}
 </div>
 `;
}
function clearTasksHistory() {
 if (!confirm(_t('tasks.clearHistoryConfirm'))) {
 return;
 }
 tasksState.completedTasksHistory = [];
 saveCompletedTasksHistory();
 filterAndSortTasks();
}
function toggleTaskSelection(conversationId, selected) {
 if (selected) {
 tasksState.selectedTasks.add(conversationId);
 } else {
 tasksState.selectedTasks.delete(conversationId);
 }
 updateBatchActions();
}
function updateBatchActions() {
 const batchActions = document.getElementById('tasks-batch-actions');
 const selectedCount = document.getElementById('tasks-selected-count');
 
 if (!batchActions || !selectedCount) return;
 
 const count = tasksState.selectedTasks.size;
 if (count > 0) {
 batchActions.style.display = 'flex';
 selectedCount.textContent = typeof window.t === 'function' ? window.t('mcp.selectedCount', { count: count }) : ` ${count} `;
 } else {
 batchActions.style.display = 'none';
 }
}
function clearTaskSelection() {
 tasksState.selectedTasks.clear();
 updateBatchActions();
 filterAndSortTasks();
}
async function batchCancelTasks() {
 const selected = Array.from(tasksState.selectedTasks);
 if (selected.length === 0) return;
 
 if (!confirm(_t('tasks.confirmCancelTasks', { n: selected.length }))) {
 return;
 }
 
 let successCount = 0;
 let failCount = 0;
 
 for (const conversationId of selected) {
 try {
 const response = await apiFetch('/api/agent-loop/cancel', {
 method: 'POST',
 headers: {
 'Content-Type': 'application/json',
 },
 body: JSON.stringify({ conversationId }),
 });
 
 if (response.ok) {
 successCount++;
 } else {
 failCount++;
 }
 } catch (error) {
 console.error('cancelTasksfailed:', conversationId, error);
 failCount++;
 }
 }
 clearTaskSelection();
 await loadTasks();
 if (failCount > 0) {
 alert(_t('tasks.batchCancelResultPartial', { success: successCount, fail: failCount }));
 } else {
 alert(_t('tasks.batchCancelResultSuccess', { n: successCount }));
 }
}
function copyTaskId(conversationId) {
 navigator.clipboard.writeText(conversationId).then(() => {
 const tooltip = document.createElement('div');
 tooltip.textContent = _t('tasks.copiedToast');
 tooltip.style.cssText = 'position: fixed; top: 50%; left: 50%; transform: translate(-50%, -50%); background: rgba(0,0,0,0.8); color: white; padding: 8px 16px; border-radius: 4px; z-index: 10000;';
 document.body.appendChild(tooltip);
 setTimeout(() => tooltip.remove(), 1000);
 }).catch(err => {
 console.error('Copy failed:', err);
 });
}
async function cancelTask(conversationId, button) {
 if (!conversationId) return;
 
 const originalText = button.textContent;
 button.disabled = true;
 button.textContent = _t('tasks.cancelling');

 try {
 const response = await apiFetch('/api/agent-loop/cancel', {
 method: 'POST',
 headers: {
 'Content-Type': 'application/json',
 },
 body: JSON.stringify({ conversationId }),
 });

 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('tasks.cancelTaskFailed'));
 }
 tasksState.selectedTasks.delete(conversationId);
 updateBatchActions();
 await loadTasks();
 } catch (error) {
 console.error('cancelTasksfailed:', error);
 alert(_t('tasks.cancelTaskFailed') + ': ' + error.message);
 button.disabled = false;
 button.textContent = originalText;
 }
}
function viewConversation(conversationId) {
 if (!conversationId) return;
 if (typeof switchPage === 'function') {
 switchPage('chat');
 setTimeout(() => {
 if (typeof loadConversation === 'function') {
 loadConversation(conversationId);
 } else if (typeof window.loadConversation === 'function') {
 window.loadConversation(conversationId);
 } else {
 window.location.hash = `chat?conversation=${conversationId}`;
 console.log('Chatpage,ChatID:', conversationId);
 }
 }, 500);
 }
}
async function refreshTasks() {
 await loadTasks();
}
function toggleTasksAutoRefresh(enabled) {
 tasksState.autoRefresh = enabled;
 localStorage.setItem('tasks-auto-refresh', enabled ? 'true' : 'false');
 
 if (enabled) {
 if (!tasksState.refreshInterval) {
 tasksState.refreshInterval = setInterval(() => {
 loadBatchQueues();
 }, 5000);
 }
 } else {
 if (tasksState.refreshInterval) {
 clearInterval(tasksState.refreshInterval);
 tasksState.refreshInterval = null;
 }
 }
}
function initTasksPage() {
 const autoRefreshCheckbox = document.getElementById('tasks-auto-refresh');
 if (autoRefreshCheckbox) {
 const saved = localStorage.getItem('tasks-auto-refresh');
 const enabled = saved !== null ? saved === 'true' : true;
 autoRefreshCheckbox.checked = enabled;
 toggleTasksAutoRefresh(enabled);
 } else {
 toggleTasksAutoRefresh(true);
 }
 loadBatchQueues();
}
function cleanupTasksPage() {
 if (tasksState.refreshInterval) {
 clearInterval(tasksState.refreshInterval);
 tasksState.refreshInterval = null;
 }
 if (tasksState.durationUpdateInterval) {
 clearInterval(tasksState.durationUpdateInterval);
 tasksState.durationUpdateInterval = null;
 }
 tasksState.selectedTasks.clear();
 stopBatchQueueRefresh();
}
window.loadTasks = loadTasks;
window.cancelTask = cancelTask;
window.viewConversation = viewConversation;
window.refreshTasks = refreshTasks;
window.initTasksPage = initTasksPage;
window.cleanupTasksPage = cleanupTasksPage;
window.filterTasks = filterTasks;
window.sortTasks = sortTasks;
window.toggleTaskSelection = toggleTaskSelection;
window.clearTaskSelection = clearTaskSelection;
window.batchCancelTasks = batchCancelTasks;
window.copyTaskId = copyTaskId;
window.toggleTasksAutoRefresh = toggleTasksAutoRefresh;
window.toggleShowHistory = toggleShowHistory;
window.clearTasksHistory = clearTasksHistory;
const batchQueuesState = {
 queues: [],
 currentQueueId: null,
 refreshInterval: null,
 filterStatus: 'all', // 'all', 'pending', 'running', 'paused', 'completed', 'cancelled'
 searchKeyword: '',
 currentPage: 1,
 pageSize: 10,
 total: 0,
 totalPages: 1
};
async function showBatchImportModal() {
 const modal = document.getElementById('batch-import-modal');
 const input = document.getElementById('batch-tasks-input');
 const titleInput = document.getElementById('batch-queue-title');
 const roleSelect = document.getElementById('batch-queue-role');
 const agentModeSelect = document.getElementById('batch-queue-agent-mode');
 const scheduleModeSelect = document.getElementById('batch-queue-schedule-mode');
 const cronExprInput = document.getElementById('batch-queue-cron-expr');
 const executeNowCheckbox = document.getElementById('batch-queue-execute-now');
 if (modal && input) {
 input.value = '';
 if (titleInput) {
 titleInput.value = '';
 }
 if (roleSelect) {
 roleSelect.value = '';
 }
 if (agentModeSelect) {
 agentModeSelect.value = 'single';
 }
 if (scheduleModeSelect) {
 scheduleModeSelect.value = 'manual';
 }
 if (cronExprInput) {
 cronExprInput.value = '';
 }
 if (executeNowCheckbox) {
 executeNowCheckbox.checked = false;
 }
 handleBatchScheduleModeChange();
 updateBatchImportStats('');
 if (roleSelect && typeof loadRoles === 'function') {
 try {
 const loadedRoles = await loadRoles();
 roleSelect.innerHTML = '<option value="">' + _t('batchImportModal.defaultRole') + '</option>';
 const sortedRoles = loadedRoles.sort((a, b) => {
 if (a.name === '') return -1;
 if (b.name === '') return 1;
 return (a.name || '').localeCompare(b.name || '', 'zh-CN');
 });
 
 sortedRoles.forEach(role => {
 if (role.name !== '' && role.enabled !== false) {
 const option = document.createElement('option');
 option.value = role.name;
 option.textContent = role.name;
 roleSelect.appendChild(option);
 }
 });
 } catch (error) {
 console.error('loadfailed:', error);
 }
 }
 
 modal.style.display = 'block';
 input.focus();
 }
}
function closeBatchImportModal() {
 const modal = document.getElementById('batch-import-modal');
 if (modal) {
 modal.style.display = 'none';
 }
}

function handleBatchScheduleModeChange() {
 const scheduleModeSelect = document.getElementById('batch-queue-schedule-mode');
 const cronGroup = document.getElementById('batch-queue-cron-group');
 const cronExprInput = document.getElementById('batch-queue-cron-expr');
 const isCron = scheduleModeSelect && scheduleModeSelect.value === 'cron';
 if (cronGroup) {
 cronGroup.style.display = isCron ? 'block' : 'none';
 }
 if (cronExprInput) {
 if (isCron) {
 cronExprInput.setAttribute('required', 'required');
 } else {
 cronExprInput.removeAttribute('required');
 cronExprInput.value = '';
 }
 }
}
function updateBatchImportStats(text) {
 const statsEl = document.getElementById('batch-import-stats');
 if (!statsEl) return;
 
 const lines = text.split('\n').filter(line => line.trim() !== '');
 const count = lines.length;
 
 if (count > 0) {
 statsEl.innerHTML = '<div class="batch-import-stat">' + _t('tasks.taskCount', { count: count }) + '</div>';
 statsEl.style.display = 'block';
 } else {
 statsEl.style.display = 'none';
 }
}
document.addEventListener('DOMContentLoaded', function() {
 const input = document.getElementById('batch-tasks-input');
 if (input) {
 input.addEventListener('input', function() {
 updateBatchImportStats(this.value);
 });
 }
});
async function createBatchQueue() {
 const input = document.getElementById('batch-tasks-input');
 const titleInput = document.getElementById('batch-queue-title');
 const roleSelect = document.getElementById('batch-queue-role');
 const agentModeSelect = document.getElementById('batch-queue-agent-mode');
 const scheduleModeSelect = document.getElementById('batch-queue-schedule-mode');
 const cronExprInput = document.getElementById('batch-queue-cron-expr');
 const executeNowCheckbox = document.getElementById('batch-queue-execute-now');
 if (!input) return;
 
 const text = input.value.trim();
 if (!text) {
 alert(_t('tasks.enterTaskPrompt'));
 return;
 }
 const tasks = text.split('\n').map(line => line.trim()).filter(line => line !== '');
 if (tasks.length === 0) {
 alert(_t('tasks.noValidTask'));
 return;
 }
 const title = titleInput ? titleInput.value.trim() : '';
 const role = roleSelect ? roleSelect.value || '' : '';
 const rawMode = agentModeSelect ? agentModeSelect.value : 'single';
 const agentMode = isBatchQueueAgentMode(rawMode) ? rawMode : 'single';
 const scheduleMode = scheduleModeSelect ? (scheduleModeSelect.value === 'cron' ? 'cron' : 'manual') : 'manual';
 const cronExpr = cronExprInput ? cronExprInput.value.trim() : '';
 const executeNow = executeNowCheckbox ? !!executeNowCheckbox.checked : false;
 if (scheduleMode === 'cron' && !cronExpr) {
 alert(_t('batchImportModal.cronExprRequired'));
 return;
 }
 if (scheduleMode === 'cron' && !/^\S+\s+\S+\s+\S+\s+\S+\s+\S+$/.test(cronExpr)) {
 alert(_t('batchImportModal.cronExprInvalid') || 'Invalid Cron expression format. Must have 5 fields (minute hour day month weekday), e.g.: 0 */2 * * *');
 return;
 }

 try {
 const response = await apiFetch('/api/batch-tasks', {
 method: 'POST',
 headers: {
 'Content-Type': 'application/json',
 },
 body: JSON.stringify({ title, tasks, role, agentMode, scheduleMode, cronExpr, executeNow }),
 });
 
 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('tasks.createBatchQueueFailed'));
 }
 
 const result = await response.json();
 closeBatchImportModal();
 showBatchQueueDetail(result.queueId);
 refreshBatchQueues();
 } catch (error) {
 console.error('createtask queuefailed:', error);
 alert(_t('tasks.createBatchQueueFailed') + ': ' + error.message);
 }
}
// Emoji-free role glyph: 1-2 letter monogram (matches roles.js roleMonogram).
function getRoleIconForDisplay(roleName, rolesList) {
 const parts = String(roleName || '').trim().split(/[ _-]+/).filter(Boolean);
 if (!parts.length) return 'DF';
 return (parts.length >= 2 ? parts[0][0] + parts[1][0] : parts[0].slice(0, 2)).toUpperCase();
}
async function loadBatchQueues(page) {
 const section = document.getElementById('batch-queues-section');
 if (!section) return;
 if (page !== undefined) {
 batchQueuesState.currentPage = page;
 }
 let loadedRoles = [];
 if (typeof loadRoles === 'function') {
 try {
 loadedRoles = await loadRoles();
 } catch (error) {
 console.warn('Failed to load role list; using the default icon:', error);
 }
 }
 batchQueuesState.loadedRoles = loadedRoles; // savestatus
 const params = new URLSearchParams();
 params.append('page', batchQueuesState.currentPage.toString());
 params.append('limit', batchQueuesState.pageSize.toString());
 if (batchQueuesState.filterStatus && batchQueuesState.filterStatus !== 'all') {
 params.append('status', batchQueuesState.filterStatus);
 }
 if (batchQueuesState.searchKeyword) {
 params.append('keyword', batchQueuesState.searchKeyword);
 }
 
 try {
 const response = await apiFetch(`/api/batch-tasks?${params.toString()}`);
 if (!response.ok) {
 throw new Error(_t('tasks.loadFailedRetry'));
 }
 
 const result = await response.json();
 batchQueuesState.queues = result.queues || [];
 batchQueuesState.total = result.total || 0;
 batchQueuesState.totalPages = result.total_pages || 1;
 renderBatchQueues();
 } catch (error) {
 console.error('loadtask queuefailed:', error);
 section.style.display = 'block';
 const list = document.getElementById('batch-queues-list');
 if (list) {
 list.innerHTML = '<div class="tasks-empty"><p>' + _t('tasks.loadFailedRetry') + ': ' + escapeHtml(error.message) + '</p><button class="btn-secondary" onclick="refreshBatchQueues()">' + _t('tasks.retry') + '</button></div>';
 }
 }
}
function filterBatchQueues() {
 const statusFilter = document.getElementById('batch-queues-status-filter');
 const searchInput = document.getElementById('batch-queues-search');
 
 if (statusFilter) {
 batchQueuesState.filterStatus = statusFilter.value;
 }
 if (searchInput) {
 batchQueuesState.searchKeyword = searchInput.value.trim();
 }
 batchQueuesState.currentPage = 1;
 loadBatchQueues(1);
}
function renderBatchQueues() {
 const section = document.getElementById('batch-queues-section');
 const list = document.getElementById('batch-queues-list');
 const pagination = document.getElementById('batch-queues-pagination');
 
 if (!section || !list) return;
 
 section.style.display = 'block';
 
 const queues = batchQueuesState.queues;
 
 if (queues.length === 0) {
 list.innerHTML = '<div class="tasks-empty"><p>' + _t('tasks.noBatchQueues') + '</p></div>';
 if (pagination) pagination.style.display = 'none';
 return;
 }
 if (pagination) {
 pagination.style.display = '';
 }
 
 list.innerHTML = queues.map(queue => {
 const pres = getBatchQueueStatusPresentation(queue);
 const stats = {
 total: queue.tasks.length,
 pending: 0,
 running: 0,
 completed: 0,
 failed: 0,
 cancelled: 0
 };
 
 queue.tasks.forEach(task => {
 if (task.status === 'pending') stats.pending++;
 else if (task.status === 'running') stats.running++;
 else if (task.status === 'completed') stats.completed++;
 else if (task.status === 'failed') stats.failed++;
 else if (task.status === 'cancelled') stats.cancelled++;
 });
 
 const progress = stats.total > 0 ? Math.round((stats.completed + stats.failed + stats.cancelled) / stats.total * 100) : 0;
 const canDelete = queue.status === 'pending' || queue.status === 'completed' || queue.status === 'cancelled';
 
 const loadedRoles = batchQueuesState.loadedRoles || [];
 const roleIcon = getRoleIconForDisplay(queue.role, loadedRoles);
 const roleName = queue.role && queue.role !== '' ? queue.role : _t('batchQueueDetailModal.defaultRole');
 const isCronCycleIdle = queue.scheduleMode === 'cron' && queue.scheduleEnabled !== false && queue.status === 'completed';
 const cardMod = isCronCycleIdle ? ' batch-queue-item--cron-wait' : '';
 const progressFillMod = isCronCycleIdle ? ' batch-queue-progress-fill--cron-wait' : '';

 const agentLabel = batchQueueAgentModeLabel(queue.agentMode);
 let scheduleLabel = queue.scheduleMode === 'cron' ? _t('batchImportModal.scheduleModeCron') : _t('batchImportModal.scheduleModeManual');
 if (queue.scheduleMode === 'cron' && queue.cronExpr) {
 scheduleLabel += ` (${queue.cronExpr})`;
 }
 const configLine = [roleName, agentLabel, scheduleLabel].map(s => escapeHtml(s)).join(' · ');
 const cronPausedNote = queue.scheduleMode === 'cron' && queue.scheduleEnabled === false
 ? ` <span class="batch-queue-inline-warn" title="${escapeHtml(_t('batchQueueDetailModal.scheduleCronAutoHint'))}">(${escapeHtml(_t('batchQueueDetailModal.cronSchedulePausedBadge'))})</span>`
 : '';
 const shortId = queue.id.length > 14 ? escapeHtml(queue.id.slice(0, 12)) + '\u2026' : escapeHtml(queue.id);
 const titleBlock = queue.title
 ? `<h4 class="batch-queue-card-title">${escapeHtml(queue.title)}</h4>`
 : `<h4 class="batch-queue-card-title batch-queue-card-title--muted">${escapeHtml(_t('tasks.batchQueueUntitled'))}</h4>`;
 const doneCount = stats.completed + stats.failed + stats.cancelled;

 const noActionsClass = canDelete ? '' : ' batch-queue-item--no-actions';
 return `
 <div class="batch-queue-item batch-queue-item--compact${cardMod}${noActionsClass}" data-queue-id="${queue.id}" onclick="showBatchQueueDetail('${queue.id}')">
 <div class="batch-queue-item__inner batch-queue-item__inner--grid">
 <div class="batch-queue-item__lead">
 <div class="batch-queue-item__title-row">
 <span class="batch-queue-item__role-icon" aria-hidden="true">${escapeHtml(roleIcon)}</span>
 <div class="batch-queue-item__titles">${titleBlock}</div>
 </div>
 <p class="batch-queue-item__config">${configLine}${cronPausedNote}</p>
 <p class="batch-queue-item__idline batch-queue-item__idline--lead"><code title="${escapeHtml(queue.id)}">${shortId}</code><span class="batch-queue-item__idsep">\u00b7</span><span>${escapeHtml(_t('tasks.createdTimeLabel'))}\u00a0${escapeHtml(new Date(queue.createdAt).toLocaleString())}</span></p>
 </div>
 <div class="batch-queue-item__cluster">
 <div class="batch-queue-item__status-inline">
 <span class="batch-queue-status ${pres.class}">${escapeHtml(pres.text)}</span>
 <span class="batch-queue-item__pct">${progress}%\u00a0<span class="batch-queue-item__pct-frac">(${doneCount}/${stats.total})</span></span>
 </div>
 ${pres.sublabel ? `<span class="batch-queue-item__sublabel">${escapeHtml(pres.sublabel)}</span>` : ''}
 </div>
 <div class="batch-queue-item__progress-col">
 <div class="batch-queue-progress-bar batch-queue-progress-bar--card batch-queue-progress-bar--list batch-queue-progress-bar--card-row">
 <div class="batch-queue-progress-fill${progressFillMod}" style="width: ${progress}%"></div>
 </div>
 </div>
 <div class="batch-queue-item__actions-col" onclick="event.stopPropagation();">
 ${canDelete ? `<button type="button" class="batch-queue-icon-btn" onclick="deleteBatchQueueFromList('${queue.id}')" title="${escapeHtml(_t('tasks.deleteQueue'))}" aria-label="${escapeHtml(_t('tasks.deleteQueue'))}"><svg class="batch-queue-icon-btn__svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/><path d="M10 11v6"/><path d="M14 11v6"/></svg></button>` : ''}
 </div>
 </div>
 </div>
 `;

 }).join('');
 renderBatchQueuesPagination();
}
function renderBatchQueuesPagination() {
 const paginationContainer = document.getElementById('batch-queues-pagination');
 if (!paginationContainer) return;
 
 const { currentPage, pageSize, total, totalPages } = batchQueuesState;
 if (total === 0) {
 paginationContainer.innerHTML = '';
 return;
 }
 const start = total === 0 ? 0 : (currentPage - 1) * pageSize + 1;
 const end = total === 0 ? 0 : Math.min(currentPage * pageSize, total);
 
 let paginationHTML = '<div class="monitor-pagination">';
 paginationHTML += `
 <div class="pagination-info">
 <span>` + _t('tasks.paginationShow', { start: start, end: end, total: total }) + `</span>
 <label class="pagination-page-size">
 ` + _t('tasks.paginationPerPage') + `
 <select id="batch-queues-page-size-pagination" onchange="changeBatchQueuesPageSize()">
 <option value="10" ${pageSize === 10 ? 'selected' : ''}>10</option>
 <option value="20" ${pageSize === 20 ? 'selected' : ''}>20</option>
 <option value="50" ${pageSize === 50 ? 'selected' : ''}>50</option>
 <option value="100" ${pageSize === 100 ? 'selected' : ''}>100</option>
 </select>
 </label>
 </div>
 `;
 paginationHTML += `
 <div class="pagination-controls">
 <button class="btn-secondary" onclick="goBatchQueuesPage(1)" ${currentPage === 1 || total === 0 ? 'disabled' : ''}>` + _t('tasks.paginationFirst') + `</button>
 <button class="btn-secondary" onclick="goBatchQueuesPage(${currentPage - 1})" ${currentPage === 1 || total === 0 ? 'disabled' : ''}>` + _t('tasks.paginationPrev') + `</button>
 <span class="pagination-page">` + _t('tasks.paginationPage', { current: currentPage, total: totalPages || 1 }) + `</span>
 <button class="btn-secondary" onclick="goBatchQueuesPage(${currentPage + 1})" ${currentPage >= totalPages || total === 0 ? 'disabled' : ''}>` + _t('tasks.paginationNext') + `</button>
 <button class="btn-secondary" onclick="goBatchQueuesPage(${totalPages || 1})" ${currentPage >= totalPages || total === 0 ? 'disabled' : ''}>` + _t('tasks.paginationLast') + `</button>
 </div>
 `;
 
 paginationHTML += '</div>';
 
 paginationContainer.innerHTML = paginationHTML;
}
function goBatchQueuesPage(page) {
 const { totalPages } = batchQueuesState;
 if (page < 1 || page > totalPages) return;
 
 loadBatchQueues(page);
 const list = document.getElementById('batch-queues-list');
 if (list) {
 list.scrollIntoView({ behavior: 'smooth', block: 'start' });
 }
}
function changeBatchQueuesPageSize() {
 const pageSizeSelect = document.getElementById('batch-queues-page-size-pagination');
 if (!pageSizeSelect) return;
 
 const newPageSize = parseInt(pageSizeSelect.value, 10);
 if (newPageSize && newPageSize > 0) {
 batchQueuesState.pageSize = newPageSize;
 batchQueuesState.currentPage = 1; // first page
 loadBatchQueues(1);
 }
}
async function showBatchQueueDetail(queueId) {
 const modal = document.getElementById('batch-queue-detail-modal');
 const title = document.getElementById('batch-queue-detail-title');
 const content = document.getElementById('batch-queue-detail-content');
 const startBtn = document.getElementById('batch-queue-start-btn');
 const cancelBtn = document.getElementById('batch-queue-cancel-btn');
 const deleteBtn = document.getElementById('batch-queue-delete-btn');
 const addTaskBtn = document.getElementById('batch-queue-add-task-btn');
 
 if (!modal || !content) return;
 
 try {
 let loadedRoles = [];
 if (typeof loadRoles === 'function') {
 try {
 loadedRoles = await loadRoles();
 } catch (error) {
 console.warn('Failed to load role list; using the default icon:', error);
 }
 }
 
 const response = await apiFetch(`/api/batch-tasks/${queueId}`);
 if (!response.ok) {
 throw new Error(_t('tasks.getQueueDetailFailed'));
 }
 
 const result = await response.json();
 const queue = result.queue;
 batchQueuesState.currentQueueId = queueId;
 const pres = getBatchQueueStatusPresentation(queue);
 const allowSubtaskMutation = batchQueueAllowsSubtaskMutation(queue);

 if (title) {
 title.textContent = queue.title ? _t('tasks.batchQueueTitle') + ' - ' + String(queue.title) : _t('tasks.batchQueueTitle');
 }
 const pauseBtn = document.getElementById('batch-queue-pause-btn');
 if (addTaskBtn) {
 addTaskBtn.style.display = allowSubtaskMutation ? 'inline-block' : 'none';
 }
 if (startBtn) {
 startBtn.style.display = (queue.status === 'pending' || queue.status === 'paused') ? 'inline-block' : 'none';
 if (startBtn && queue.status === 'paused') {
 startBtn.textContent = _t('tasks.resumeExecute');
 } else if (startBtn && queue.status === 'pending') {
 const isCronPending = queue.scheduleMode === 'cron' && queue.scheduleEnabled !== false;
 startBtn.textContent = isCronPending
 ? _t('batchQueueDetailModal.startExecuteNow')
 : _t('batchQueueDetailModal.startExecute');
 }
 }
 const rerunBtn = document.getElementById('batch-queue-rerun-btn');
 if (rerunBtn) {
 rerunBtn.style.display = (queue.status === 'completed' || queue.status === 'cancelled') ? 'inline-block' : 'none';
 }
 if (pauseBtn) {
 pauseBtn.style.display = queue.status === 'running' ? 'inline-block' : 'none';
 }
 if (deleteBtn) {
 deleteBtn.style.display = (queue.status === 'pending' || queue.status === 'completed' || queue.status === 'cancelled' || queue.status === 'paused') ? 'inline-block' : 'none';
 }
 const taskStatusMap = {
 'pending': { text: _t('tasks.statusPending'), class: 'batch-task-status-pending' },
 'running': { text: _t('tasks.statusRunning'), class: 'batch-task-status-running' },
 'completed': { text: _t('tasks.statusCompleted'), class: 'batch-task-status-completed' },
 'failed': { text: _t('tasks.failedLabel'), class: 'batch-task-status-failed' },
 'cancelled': { text: _t('tasks.statusCancelled'), class: 'batch-task-status-cancelled' }
 };
 
 let roleLineVal = '';
 if (queue.role && queue.role !== '') {
 let roleName = queue.role;
 let roleIcon = '\uD83D\uDC64';
 if (Array.isArray(loadedRoles) && loadedRoles.length > 0) {
 const role = loadedRoles.find(r => r.name === roleName);
 if (role && role.icon) {
 let icon = role.icon;
 const unicodeMatch = icon.match(/^"?\\U([0-9A-F]{8})"?$/i);
 if (unicodeMatch) {
 try {
 const codePoint = parseInt(unicodeMatch[1], 16);
 icon = String.fromCodePoint(codePoint);
 } catch (e) {
 // ignore
 }
 }
 roleIcon = icon;
 }
 }
 roleLineVal = roleIcon + ' ' + escapeHtml(roleName);
 } else {
 roleLineVal = '\uD83D\uDD35 ' + escapeHtml(_t('batchQueueDetailModal.defaultRole'));
 }
 const agentModeText = batchQueueAgentModeLabel(queue.agentMode);
 const scheduleModeText = queue.scheduleMode === 'cron' ? _t('batchImportModal.scheduleModeCron') : _t('batchImportModal.scheduleModeManual');
 const scheduleDetail = escapeHtml(scheduleModeText) + (queue.scheduleMode === 'cron' && queue.cronExpr ? `(${escapeHtml(queue.cronExpr)})` : '');
 const showProgressNoteInModal = !!(pres.progressNote && !pres.callout);
 const modalBody = content.closest('.modal-body');
 const tasksList = content.querySelector('.batch-queue-tasks-list');
 const savedModalBodyScrollTop = modalBody ? modalBody.scrollTop : 0;
 const savedTasksListScrollTop = tasksList ? tasksList.scrollTop : 0;
 const prevTechDetails = content.querySelector('details.batch-queue-detail-tech');
 const prevLayout = content.querySelector('.batch-queue-detail-layout[data-bq-detail-for]');
 const prevDetailFor = prevLayout ? prevLayout.getAttribute('data-bq-detail-for') : null;
 const sameQueueAsBefore = prevDetailFor === queue.id;
 const savedTechDetailsOpen = sameQueueAsBefore && !!(prevTechDetails && prevTechDetails.open);

 content.innerHTML = `
 <div class="batch-queue-detail-layout" data-bq-detail-for="${escapeHtml(queue.id)}">
 <section class="batch-queue-detail-hero">
 <span class="batch-queue-status ${pres.class}">${escapeHtml(pres.text)}</span>
 ${pres.sublabel ? `<p class="batch-queue-detail-hero__sub">${escapeHtml(pres.sublabel)}</p>` : ''}
 ${showProgressNoteInModal ? `<p class="batch-queue-detail-hero__note">${escapeHtml(pres.progressNote)}</p>` : ''}
 </section>
 <section class="batch-queue-detail-kv">
 <div class="bq-kv"><span class="bq-kv__k">${escapeHtml(_t('batchQueueDetailModal.queueTitle'))}</span><span class="bq-kv__v" id="bq-title-val">${allowSubtaskMutation ? `<span class="bq-inline-editable" onclick="startInlineEditTitle()" title="${escapeHtml(_t('common.edit'))}">${escapeHtml(queue.title || _t('tasks.batchQueueUntitled'))}</span>` : escapeHtml(queue.title || _t('tasks.batchQueueUntitled'))}</span></div>
 <div class="bq-kv"><span class="bq-kv__k">${escapeHtml(_t('batchQueueDetailModal.role'))}</span><span class="bq-kv__v" id="bq-role-val">${allowSubtaskMutation ? `<span class="bq-inline-editable" onclick="startInlineEditRole()" title="${escapeHtml(_t('common.edit'))}">${roleLineVal}</span>` : roleLineVal}</span></div>
 <div class="bq-kv"><span class="bq-kv__k">${escapeHtml(_t('batchImportModal.agentMode'))}</span><span class="bq-kv__v" id="bq-agentmode-val">${allowSubtaskMutation ? `<span class="bq-inline-editable" onclick="startInlineEditAgentMode()" title="${escapeHtml(_t('common.edit'))}">${escapeHtml(agentModeText)}</span>` : escapeHtml(agentModeText)}</span></div>
 <div class="bq-kv"><span class="bq-kv__k">${escapeHtml(_t('batchImportModal.scheduleMode'))}</span><span class="bq-kv__v" id="bq-schedule-val">${allowSubtaskMutation ? `<span class="bq-inline-editable" onclick="startInlineEditSchedule()" title="${escapeHtml(_t('common.edit'))}">${scheduleDetail}</span>` : scheduleDetail}</span></div>
 <div class="bq-kv"><span class="bq-kv__k">${escapeHtml(_t('batchQueueDetailModal.taskTotal'))}</span><span class="bq-kv__v">${queue.tasks.length}</span></div>
 ${queue.scheduleMode === 'cron' ? `<div class="bq-kv bq-kv--block"><span class="bq-kv__k">${escapeHtml(_t('batchQueueDetailModal.scheduleCronAuto'))}</span><span class="bq-kv__v bq-kv__v--control"><label class="bq-cron-toggle"><input type="checkbox" ${queue.scheduleEnabled !== false ? 'checked' : ''} onchange="updateBatchQueueScheduleEnabled(this.checked)" /><span class="bq-cron-toggle__hint">${escapeHtml(_t('batchQueueDetailModal.scheduleCronAutoHint'))}</span></label></span></div>` : ''}
 </section>
 ${queue.lastScheduleError ? `<div class="bq-alert bq-alert--err"><strong>${escapeHtml(_t('batchQueueDetailModal.lastScheduleError'))}</strong><p>${escapeHtml(queue.lastScheduleError)}</p></div>` : ''}
 ${queue.lastRunError ? `<div class="bq-alert bq-alert--err"><strong>${escapeHtml(_t('batchQueueDetailModal.lastRunError'))}</strong><p>${escapeHtml(queue.lastRunError)}</p></div>` : ''}
 ${pres.callout ? `<div class="batch-queue-cron-callout batch-queue-cron-callout--compact"><span class="batch-queue-cron-callout-icon" aria-hidden="true">\u21BB</span><p>${escapeHtml(pres.callout)}</p></div>` : ''}
 <details class="batch-queue-detail-tech">
 <summary class="batch-queue-detail-tech__sum">${escapeHtml(_t('batchQueueDetailModal.technicalDetails'))}</summary>
 <div class="batch-queue-detail-tech__body">
 <div class="bq-kv"><span class="bq-kv__k">${escapeHtml(_t('batchQueueDetailModal.queueId'))}</span><span class="bq-kv__v"><code>${escapeHtml(queue.id)}</code></span></div>
 <div class="bq-kv"><span class="bq-kv__k">${escapeHtml(_t('batchQueueDetailModal.createdAt'))}</span><span class="bq-kv__v">${escapeHtml(new Date(queue.createdAt).toLocaleString())}</span></div>
 ${queue.startedAt ? `<div class="bq-kv"><span class="bq-kv__k">${escapeHtml(_t('batchQueueDetailModal.startedAt'))}</span><span class="bq-kv__v">${escapeHtml(new Date(queue.startedAt).toLocaleString())}</span></div>` : ''}
 ${queue.completedAt ? `<div class="bq-kv"><span class="bq-kv__k">${escapeHtml(_t('batchQueueDetailModal.completedAt'))}</span><span class="bq-kv__v">${escapeHtml(new Date(queue.completedAt).toLocaleString())}</span></div>` : ''}
 ${queue.scheduleMode === 'cron' && queue.nextRunAt && !pres.sublabel ? `<div class="bq-kv"><span class="bq-kv__k">${escapeHtml(_t('batchQueueDetailModal.nextRunAt'))}</span><span class="bq-kv__v">${escapeHtml(new Date(queue.nextRunAt).toLocaleString())}</span></div>` : ''}
 ${queue.lastScheduleTriggerAt ? `<div class="bq-kv"><span class="bq-kv__k">${escapeHtml(_t('batchQueueDetailModal.lastScheduleTriggerAt'))}</span><span class="bq-kv__v">${escapeHtml(new Date(queue.lastScheduleTriggerAt).toLocaleString())}</span></div>` : ''}
 </div>
 </details>
 </div>
 <div class="batch-queue-tasks-list">
 <h4>` + _t('batchQueueDetailModal.taskList') + `</h4>
 ${queue.tasks.map((task, index) => {
 const taskStatus = taskStatusMap[task.status] || { text: task.status, class: 'batch-task-status-unknown' };
 const canEdit = allowSubtaskMutation && task.status !== 'running';
 const taskMessageEscaped = escapeHtml(task.message).replace(/'/g, "&#39;").replace(/"/g, "&quot;").replace(/\n/g, "\\n");
 return `
 <div class="batch-task-item ${task.status === 'running' ? 'batch-task-item-active' : ''}" data-queue-id="${queue.id}" data-task-id="${task.id}" data-task-message="${taskMessageEscaped}">
 <div class="batch-task-header">
 <span class="batch-task-index">#${index + 1}</span>
 <span class="batch-task-status ${taskStatus.class}">${taskStatus.text}</span>
 <span class="batch-task-message" title="${escapeHtml(task.message)}">${escapeHtml(task.message)}</span>
 ${canEdit ? `<button class="btn-secondary btn-small batch-task-edit-btn" onclick="editBatchTaskFromElement(this); event.stopPropagation();">` + _t('common.edit') + `</button>` : ''}
 ${canEdit ? `<button class="btn-secondary btn-small btn-danger batch-task-delete-btn" onclick="deleteBatchTaskFromElement(this); event.stopPropagation();">` + _t('common.delete') + `</button>` : ''}
 ${allowSubtaskMutation && task.status === 'failed' ? `<button class="btn-secondary btn-small" onclick="retryBatchTask('${queue.id}', '${task.id}'); event.stopPropagation();">` + _t('tasks.retryTask') + `</button>` : ''}
 ${task.conversationId ? `<button class="btn-secondary btn-small" onclick="viewBatchTaskConversation('${task.conversationId}'); event.stopPropagation();">` + _t('tasks.viewConversation') + `</button>` : ''}
 </div>
 ${task.startedAt ? `<div class="batch-task-time">` + _t('batchQueueDetailModal.startLabel') + `: ${new Date(task.startedAt).toLocaleString()}</div>` : ''}
 ${task.completedAt ? `<div class="batch-task-time">` + _t('batchQueueDetailModal.completeLabel') + `: ${new Date(task.completedAt).toLocaleString()}</div>` : ''}
 ${task.error ? `<div class="batch-task-error">` + _t('batchQueueDetailModal.errorLabel') + `: ${escapeHtml(task.error)}</div>` : ''}
 ${task.result ? `<div class="batch-task-result">` + _t('batchQueueDetailModal.resultLabel') + `: ${escapeHtml(task.result.substring(0, 200))}${task.result.length > 200 ? '...' : ''}</div>` : ''}
 </div>
 `;
 }).join('')}
 </div>
 `;
 if (savedModalBodyScrollTop > 0 && modalBody) {
 modalBody.scrollTop = savedModalBodyScrollTop;
 }
 const newTasksList = content.querySelector('.batch-queue-tasks-list');
 if (savedTasksListScrollTop > 0 && newTasksList) {
 newTasksList.scrollTop = savedTasksListScrollTop;
 }

 const newTechDetails = content.querySelector('details.batch-queue-detail-tech');
 if (newTechDetails && savedTechDetailsOpen) {
 newTechDetails.open = true;
 }

 modal.style.display = 'block';
 if (queue.status === 'running') {
 startBatchQueueRefresh(queueId);
 } else {
 stopBatchQueueRefresh();
 }
 } catch (error) {
 console.error('fetchdetailsfailed:', error);
 alert(_t('tasks.getQueueDetailFailed') + ': ' + error.message);
 }
}
async function startBatchQueue() {
 const queueId = batchQueuesState.currentQueueId;
 if (!queueId) return;
 const btn = document.getElementById('batch-queue-start-btn');
 if (btn) { btn.disabled = true; }
 try {
 const queueResponse = await apiFetch(`/api/batch-tasks/${queueId}`);
 if (!queueResponse.ok) {
 throw new Error(_t('tasks.getQueueDetailFailed'));
 }
 const queueResult = await queueResponse.json();
 const queue = queueResult && queueResult.queue ? queueResult.queue : null;
 const isCronPending = queue && queue.status === 'pending' && queue.scheduleMode === 'cron' && queue.scheduleEnabled !== false;
 if (isCronPending) {
 const okNow = confirm(_t('batchQueueDetailModal.startExecuteNowConfirm'));
 if (!okNow) return;
 }

 const response = await apiFetch(`/api/batch-tasks/${queueId}/start`, {
 method: 'POST',
 });
 
 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('tasks.startBatchQueueFailed'));
 }
 showBatchQueueDetail(queueId);
 refreshBatchQueues();
 } catch (error) {
 console.error('Tasksfailed:', error);
 alert(_t('tasks.startBatchQueueFailed') + ': ' + error.message);
 } finally {
 if (btn) { btn.disabled = false; }
 }
}
async function pauseBatchQueue() {
 const queueId = batchQueuesState.currentQueueId;
 if (!queueId) return;

 if (!confirm(_t('tasks.pauseQueueConfirm'))) {
 return;
 }
 const btn = document.getElementById('batch-queue-pause-btn');
 if (btn) { btn.disabled = true; }
 try {
 const response = await apiFetch(`/api/batch-tasks/${queueId}/pause`, {
 method: 'POST',
 });
 
 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('tasks.pauseQueueFailed'));
 }
 showBatchQueueDetail(queueId);
 refreshBatchQueues();
 } catch (error) {
 console.error('Tasksfailed:', error);
 alert(_t('tasks.pauseQueueFailed') + ': ' + error.message);
 } finally {
 if (btn) { btn.disabled = false; }
 }
}
async function rerunBatchQueue() {
 const queueId = batchQueuesState.currentQueueId;
 if (!queueId) return;

 if (!confirm(_t('tasks.rerunQueueConfirm'))) {
 return;
 }
 const btn = document.getElementById('batch-queue-rerun-btn');
 if (btn) { btn.disabled = true; }
 try {
 const response = await apiFetch(`/api/batch-tasks/${queueId}/rerun`, {
 method: 'POST',
 });

 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('tasks.rerunQueueFailed'));
 }

 showBatchQueueDetail(queueId);
 refreshBatchQueues();
 } catch (error) {
 console.error('Tasksfailed:', error);
 alert(_t('tasks.rerunQueueFailed') + ': ' + error.message);
 } finally {
 if (btn) { btn.disabled = false; }
 }
}
async function deleteBatchQueue() {
 const queueId = batchQueuesState.currentQueueId;
 if (!queueId) return;

 if (!confirm(_t('tasks.deleteQueueConfirm'))) {
 return;
 }
 const btn = document.getElementById('batch-queue-delete-btn');
 if (btn) { btn.disabled = true; }
 try {
 const response = await apiFetch(`/api/batch-tasks/${queueId}`, {
 method: 'DELETE',
 });
 
 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('tasks.deleteQueueFailed'));
 }
 
 closeBatchQueueDetailModal();
 refreshBatchQueues();
 } catch (error) {
 console.error('Failed to delete batch task queue:', error);
 alert(_t('tasks.deleteQueueFailed') + ': ' + error.message);
 } finally {
 if (btn) { btn.disabled = false; }
 }
}
async function deleteBatchQueueFromList(queueId) {
 if (!queueId) return;
 
 if (!confirm(_t('tasks.deleteQueueConfirm'))) {
 return;
 }
 
 try {
 const response = await apiFetch(`/api/batch-tasks/${queueId}`, {
 method: 'DELETE',
 });
 
 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('tasks.deleteQueueFailed'));
 }
 if (batchQueuesState.currentQueueId === queueId) {
 closeBatchQueueDetailModal();
 }
 refreshBatchQueues();
 } catch (error) {
 console.error('Failed to delete batch task queue:', error);
 alert(_t('tasks.deleteQueueFailed') + ': ' + error.message);
 }
}
function closeBatchQueueDetailModal() {
 const modal = document.getElementById('batch-queue-detail-modal');
 if (modal) {
 modal.style.display = 'none';
 }
 batchQueuesState.currentQueueId = null;
 stopBatchQueueRefresh();
}
function startBatchQueueRefresh(queueId) {
 if (batchQueuesState.refreshInterval) {
 clearInterval(batchQueuesState.refreshInterval);
 }

 batchQueuesState.refreshInterval = setInterval(() => {
 const addModal = document.getElementById('add-batch-task-modal');
 const content = document.getElementById('batch-queue-detail-content');
 const hasInlineEdit = content && (
 content.querySelector('.bq-inline-edit-controls') ||
 content.querySelector('.batch-task-inline-edit')
 );
 if ((addModal && addModal.style.display === 'block') || hasInlineEdit) {
 return;
 }
 if (batchQueuesState.currentQueueId === queueId) {
 showBatchQueueDetail(queueId);
 refreshBatchQueues();
 } else {
 stopBatchQueueRefresh();
 }
 }, 3000); // 3 secondsrefresh
}
function stopBatchQueueRefresh() {
 if (batchQueuesState.refreshInterval) {
 clearInterval(batchQueuesState.refreshInterval);
 batchQueuesState.refreshInterval = null;
 }
}
async function refreshBatchQueues() {
 await loadBatchQueues(batchQueuesState.currentPage);
}
function viewBatchTaskConversation(conversationId) {
 if (!conversationId) return;
 closeBatchQueueDetailModal();
 window.location.hash = `chat?conversation=${conversationId}`;
}
function editBatchTaskFromElement(button) {
 const taskItem = button.closest('.batch-task-item');
 if (!taskItem) return;

 const queueId = taskItem.getAttribute('data-queue-id');
 const taskId = taskItem.getAttribute('data-task-id');
 const taskMessage = taskItem.getAttribute('data-task-message');
 if (!queueId || !taskId) return;
 const decodedMessage = taskMessage
 .replace(/&#39;/g, "'")
 .replace(/&quot;/g, '"')
 .replace(/\\n/g, '\n');
 const msgSpan = taskItem.querySelector('.batch-task-message');
 const header = taskItem.querySelector('.batch-task-header');
 if (!msgSpan || !header) return;
 header.querySelectorAll('.batch-task-edit-btn, .batch-task-delete-btn').forEach(b => b.style.display = 'none');
 const editDiv = document.createElement('div');
 editDiv.className = 'batch-task-inline-edit';
 editDiv.innerHTML = `<textarea id="bq-task-edit-${escapeHtml(taskId)}">${escapeHtml(decodedMessage)}</textarea>`;
 msgSpan.style.display = 'none';
 msgSpan.parentNode.insertBefore(editDiv, msgSpan.nextSibling);

 const textarea = editDiv.querySelector('textarea');
 if (textarea) {
 let taskCancelled = false;
 textarea.focus();
 textarea.setSelectionRange(textarea.value.length, textarea.value.length);
 textarea.addEventListener('keydown', (e) => {
 if (e.key === 'Escape') {
 taskCancelled = true;
 cancelInlineTask();
 }
 });
 textarea.addEventListener('blur', () => {
 if (!taskCancelled) saveInlineTask(queueId, taskId);
 });
 }
}

function cancelInlineTask() {
 const queueId = batchQueuesState.currentQueueId;
 if (queueId) showBatchQueueDetail(queueId);
}

async function saveInlineTask(queueId, taskId) {
 if (_bqInlineSaving) return;
 _bqInlineSaving = true;
 const textarea = document.getElementById(`bq-task-edit-${taskId}`);
 if (!textarea) { _bqInlineSaving = false; return; }

 const message = textarea.value.trim();
 if (!message) {
 _bqInlineSaving = false;
 alert(_t('tasks.taskMessageRequired'));
 return;
 }

 try {
 const response = await apiFetch(`/api/batch-tasks/${queueId}/tasks/${taskId}`, {
 method: 'PUT',
 headers: { 'Content-Type': 'application/json' },
 body: JSON.stringify({ message }),
 });

 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('tasks.updateTaskFailed'));
 }

 _bqInlineSaving = false;
 if (batchQueuesState.currentQueueId === queueId) {
 showBatchQueueDetail(queueId);
 }
 refreshBatchQueues();
 } catch (error) {
 _bqInlineSaving = false;
 console.error('saveTasksfailed:', error);
 alert(_t('tasks.saveTaskFailed') + ': ' + error.message);
 }
}
function showAddBatchTaskModal() {
 const queueId = batchQueuesState.currentQueueId;
 if (!queueId) {
 alert(_t('tasks.queueInfoMissing'));
 return;
 }
 
 const modal = document.getElementById('add-batch-task-modal');
 const messageInput = document.getElementById('add-task-message');
 
 if (!modal || !messageInput) {
 console.error('Tasksdoes not exist');
 return;
 }
 
 messageInput.value = '';
 modal.style.display = 'block';
 setTimeout(() => {
 messageInput.focus();
 }, 100);
 if (showAddBatchTaskModal._escHandler) {
 document.removeEventListener('keydown', showAddBatchTaskModal._escHandler);
 }
 if (showAddBatchTaskModal._saveHandler && messageInput) {
 messageInput.removeEventListener('keydown', showAddBatchTaskModal._saveHandler);
 }
 showAddBatchTaskModal._escHandler = (e) => {
 if (e.key === 'Escape') {
 closeAddBatchTaskModal();
 }
 };
 document.addEventListener('keydown', showAddBatchTaskModal._escHandler);
 showAddBatchTaskModal._saveHandler = (e) => {
 if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
 e.preventDefault();
 saveAddBatchTask();
 }
 };
 messageInput.addEventListener('keydown', showAddBatchTaskModal._saveHandler);
}
function closeAddBatchTaskModal() {
 if (showAddBatchTaskModal._escHandler) {
 document.removeEventListener('keydown', showAddBatchTaskModal._escHandler);
 showAddBatchTaskModal._escHandler = null;
 }
 if (showAddBatchTaskModal._saveHandler) {
 const messageInput = document.getElementById('add-task-message');
 if (messageInput) {
 messageInput.removeEventListener('keydown', showAddBatchTaskModal._saveHandler);
 }
 showAddBatchTaskModal._saveHandler = null;
 }
 const modal = document.getElementById('add-batch-task-modal');
 const messageInput = document.getElementById('add-task-message');
 if (modal) {
 modal.style.display = 'none';
 }
 if (messageInput) {
 messageInput.value = '';
 }
}
async function saveAddBatchTask() {
 const queueId = batchQueuesState.currentQueueId;
 const messageInput = document.getElementById('add-task-message');
 
 if (!queueId) {
 alert(_t('tasks.queueInfoMissing'));
 return;
 }
 
 if (!messageInput) {
 alert(_t('tasks.cannotGetTaskMessageInput'));
 return;
 }
 
 const message = messageInput.value.trim();
 if (!message) {
 alert(_t('tasks.taskMessageRequired'));
 return;
 }
 
 try {
 const response = await apiFetch(`/api/batch-tasks/${queueId}/tasks`, {
 method: 'POST',
 headers: {
 'Content-Type': 'application/json',
 },
 body: JSON.stringify({ message: message }),
 });
 
 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('tasks.addTaskFailed'));
 }
 closeAddBatchTaskModal();
 if (batchQueuesState.currentQueueId === queueId) {
 showBatchQueueDetail(queueId);
 }
 refreshBatchQueues();
 } catch (error) {
 console.error('Tasksfailed:', error);
 alert(_t('tasks.addTaskFailed') + ': ' + error.message);
 }
}
function deleteBatchTaskFromElement(button) {
 const taskItem = button.closest('.batch-task-item');
 if (!taskItem) {
 console.error('Tasks');
 return;
 }
 
 const queueId = taskItem.getAttribute('data-queue-id');
 const taskId = taskItem.getAttribute('data-task-id');
 const taskMessage = taskItem.getAttribute('data-task-message');
 
 if (!queueId || !taskId) {
 console.error('Tasks');
 return;
 }
 const decodedMessage = taskMessage
 .replace(/&#39;/g, "'")
 .replace(/&quot;/g, '"')
 .replace(/\\n/g, '\n');
 const displayMessage = decodedMessage.length > 50 
 ? decodedMessage.substring(0, 50) + '...' 
 : decodedMessage;
 
 if (!confirm(_t('tasks.confirmDeleteTask', { message: displayMessage }))) {
 return;
 }
 
 deleteBatchTask(queueId, taskId);
}
async function deleteBatchTask(queueId, taskId) {
 if (!queueId || !taskId) {
 alert(_t('tasks.taskIncomplete'));
 return;
 }
 
 try {
 const response = await apiFetch(`/api/batch-tasks/${queueId}/tasks/${taskId}`, {
 method: 'DELETE',
 });
 
 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('tasks.deleteTaskFailed'));
 }
 if (batchQueuesState.currentQueueId === queueId) {
 showBatchQueueDetail(queueId);
 }
 refreshBatchQueues();
 } catch (error) {
 console.error('deleteTasksfailed:', error);
 alert(_t('tasks.deleteTaskFailed') + ': ' + error.message);
 }
}

async function updateBatchQueueScheduleEnabled(enabled) {
 const queueId = batchQueuesState.currentQueueId;
 if (!queueId) return;
 try {
 const response = await apiFetch(`/api/batch-tasks/${queueId}/schedule-enabled`, {
 method: 'PUT',
 headers: { 'Content-Type': 'application/json' },
 body: JSON.stringify({ scheduleEnabled: enabled }),
 });
 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('batchQueueDetailModal.scheduleToggleFailed'));
 }
 showBatchQueueDetail(queueId);
 refreshBatchQueues();
 } catch (e) {
 console.error(e);
 alert(_t('batchQueueDetailModal.scheduleToggleFailed') + ': ' + e.message);
 showBatchQueueDetail(queueId);
 }
}
function cancelAllInlineEdits() {
 _bqInlineSaving = true; // blur triggersave
 const queueId = batchQueuesState.currentQueueId;
 if (queueId) showBatchQueueDetail(queueId);
 _bqInlineSaving = false;
}
let _bqInlineSaving = false;
function startInlineEditTitle() {
 const container = document.getElementById('bq-title-val');
 if (!container) return;
 const queueId = batchQueuesState.currentQueueId;
 if (!queueId) return;
 const currentTitle = (container.querySelector('.bq-inline-editable') || container).textContent.trim();
 const untitledText = _t('tasks.batchQueueUntitled');
 const val = currentTitle === untitledText ? '' : currentTitle;
 container.innerHTML = `<span class="bq-inline-edit-controls">
 <input type="text" id="bq-edit-title" value="${escapeHtml(val)}" placeholder="${escapeHtml(_t('batchImportModal.queueTitleHint') || 'Set a title for the batch task queue to make it easier to find and manage later.')}" style="width:180px;" />
 </span>`;
 const inp = document.getElementById('bq-edit-title');
 if (inp) {
 inp.focus();
 inp.select();
 let cancelled = false;
 inp.addEventListener('keydown', (e) => {
 if (e.key === 'Enter') { e.preventDefault(); inp.blur(); }
 if (e.key === 'Escape') { cancelled = true; cancelAllInlineEdits(); }
 });
 inp.addEventListener('blur', () => {
 if (cancelled) return;
 saveInlineTitle();
 });
 }
}
async function saveInlineTitle() {
 if (_bqInlineSaving) return;
 _bqInlineSaving = true;
 const queueId = batchQueuesState.currentQueueId;
 if (!queueId) { _bqInlineSaving = false; return; }
 const inp = document.getElementById('bq-edit-title');
 const title = inp ? inp.value.trim() : '';
 try {
 const detailResp = await apiFetch(`/api/batch-tasks/${queueId}`);
 const detail = await detailResp.json();
 const role = detail.queue ? (detail.queue.role || '') : '';
 const response = await apiFetch(`/api/batch-tasks/${queueId}/metadata`, {
 method: 'PUT',
 headers: { 'Content-Type': 'application/json' },
 body: JSON.stringify({ title, role }),
 });
 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('tasks.updateTaskFailed'));
 }
 _bqInlineSaving = false;
 showBatchQueueDetail(queueId);
 refreshBatchQueues();
 } catch (e) {
 _bqInlineSaving = false;
 console.error(e);
 alert(e.message);
 }
}
function startInlineEditRole() {
 const container = document.getElementById('bq-role-val');
 if (!container) return;
 const queueId = batchQueuesState.currentQueueId;
 if (!queueId) return;
 apiFetch(`/api/batch-tasks/${queueId}`).then(r => r.json()).then(detail => {
 const queue = detail.queue;
 const currentRole = queue.role || '';
 const roles = (Array.isArray(batchQueuesState.loadedRoles) ? batchQueuesState.loadedRoles : []).filter(r => r.name !== '' && r.enabled !== false).sort((a, b) => (a.name || '').localeCompare(b.name || '', 'zh-CN'));
 const currentInList = !currentRole || roles.some(r => r.name === currentRole);
 const orphanOpt = !currentInList ? `<option value="${escapeHtml(currentRole)}" selected>${escapeHtml(currentRole)} (${escapeHtml(_t('batchQueueDetailModal.roleNotFound') || '')})</option>` : '';
 const opts = roles.map(r => `<option value="${escapeHtml(r.name)}" ${r.name === currentRole ? 'selected' : ''}>${escapeHtml(r.name)}</option>`).join('');
 container.innerHTML = `<span class="bq-inline-edit-controls">
 <select id="bq-edit-role">
 <option value="">${escapeHtml(_t('batchImportModal.defaultRole'))}</option>
 ${orphanOpt}${opts}
 </select>
 </span>`;
 const sel = document.getElementById('bq-edit-role');
 if (sel) {
 sel.focus();
 let cancelled = false;
 sel.addEventListener('keydown', (e) => {
 if (e.key === 'Escape') { cancelled = true; cancelAllInlineEdits(); }
 });
 sel.addEventListener('change', () => { if (!cancelled) saveInlineRole(); });
 sel.addEventListener('blur', () => { if (!cancelled) saveInlineRole(); });
 }
 });
}
async function saveInlineRole() {
 if (_bqInlineSaving) return;
 _bqInlineSaving = true;
 const queueId = batchQueuesState.currentQueueId;
 if (!queueId) { _bqInlineSaving = false; return; }
 const sel = document.getElementById('bq-edit-role');
 const role = sel ? sel.value.trim() : '';
 try {
 const detailResp = await apiFetch(`/api/batch-tasks/${queueId}`);
 const detail = await detailResp.json();
 const title = detail.queue ? (detail.queue.title || '') : '';
 const response = await apiFetch(`/api/batch-tasks/${queueId}/metadata`, {
 method: 'PUT',
 headers: { 'Content-Type': 'application/json' },
 body: JSON.stringify({ title, role }),
 });
 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('tasks.updateTaskFailed'));
 }
 _bqInlineSaving = false;
 showBatchQueueDetail(queueId);
 refreshBatchQueues();
 } catch (e) {
 _bqInlineSaving = false;
 console.error(e);
 alert(e.message);
 }
}
function startInlineEditAgentMode() {
 const container = document.getElementById('bq-agentmode-val');
 if (!container) return;
 const queueId = batchQueuesState.currentQueueId;
 if (!queueId) return;
 apiFetch(`/api/batch-tasks/${queueId}`).then(r => r.json()).then(detail => {
 const queue = detail.queue;
 let currentMode = (queue.agentMode || 'single').toLowerCase();
 if (currentMode === 'multi') currentMode = 'deep';
 if (!isBatchQueueAgentMode(currentMode)) currentMode = 'single';
 container.innerHTML = `<span class="bq-inline-edit-controls">
 <select id="bq-edit-agentmode">
 <option value="single" ${currentMode === 'single' ? 'selected' : ''}>${escapeHtml(_t('chat.agentModeReactNative'))}</option>
 <option value="eino_single" ${currentMode === 'eino_single' ? 'selected' : ''}>${escapeHtml(_t('chat.agentModeEinoSingle'))}</option>
 <option value="deep" ${currentMode === 'deep' ? 'selected' : ''}>${escapeHtml(_t('chat.agentModeDeep'))}</option>
 <option value="plan_execute" ${currentMode === 'plan_execute' ? 'selected' : ''}>${escapeHtml(_t('chat.agentModePlanExecuteLabel'))}</option>
 <option value="supervisor" ${currentMode === 'supervisor' ? 'selected' : ''}>${escapeHtml(_t('chat.agentModeSupervisorLabel'))}</option>
 </select>
 </span>`;
 const sel = document.getElementById('bq-edit-agentmode');
 if (sel) {
 sel.focus();
 let cancelled = false;
 sel.addEventListener('keydown', (e) => {
 if (e.key === 'Escape') { cancelled = true; cancelAllInlineEdits(); }
 });
 sel.addEventListener('change', () => { if (!cancelled) saveInlineAgentMode(); });
 sel.addEventListener('blur', () => { if (!cancelled) saveInlineAgentMode(); });
 }
 });
}
async function saveInlineAgentMode() {
 if (_bqInlineSaving) return;
 _bqInlineSaving = true;
 const queueId = batchQueuesState.currentQueueId;
 if (!queueId) { _bqInlineSaving = false; return; }
 const sel = document.getElementById('bq-edit-agentmode');
 const raw = sel ? sel.value : 'single';
 const agentMode = isBatchQueueAgentMode(raw) ? raw : 'single';
 try {
 const detailResp = await apiFetch(`/api/batch-tasks/${queueId}`);
 const detail = await detailResp.json();
 const title = detail.queue ? (detail.queue.title || '') : '';
 const role = detail.queue ? (detail.queue.role || '') : '';
 const response = await apiFetch(`/api/batch-tasks/${queueId}/metadata`, {
 method: 'PUT',
 headers: { 'Content-Type': 'application/json' },
 body: JSON.stringify({ title, role, agentMode }),
 });
 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('tasks.updateTaskFailed'));
 }
 _bqInlineSaving = false;
 showBatchQueueDetail(queueId);
 refreshBatchQueues();
 } catch (e) {
 _bqInlineSaving = false;
 console.error(e);
 alert(e.message);
 }
}
async function retryBatchTask(queueId, taskId) {
 if (!queueId || !taskId) return;
 try {
 const detailResp = await apiFetch(`/api/batch-tasks/${queueId}`);
 if (!detailResp.ok) throw new Error(_t('tasks.getQueueDetailFailed'));
 const detail = await detailResp.json();
 const task = detail.queue.tasks.find(t => t.id === taskId);
 if (!task) throw new Error(_t('tasks.taskNotFound') || 'Task not found');
 const message = task.message;
 const addResp = await apiFetch(`/api/batch-tasks/${queueId}/tasks`, {
 method: 'POST',
 headers: { 'Content-Type': 'application/json' },
 body: JSON.stringify({ message }),
 });
 if (!addResp.ok) {
 const r = await addResp.json().catch(() => ({}));
 throw new Error(r.error || _t('tasks.addTaskFailed'));
 }
 const delResp = await apiFetch(`/api/batch-tasks/${queueId}/tasks/${taskId}`, { method: 'DELETE' });
 if (!delResp.ok) {
 console.warn('deleteTasksfailed,Tasks');
 }
 showBatchQueueDetail(queueId);
 refreshBatchQueues();
 } catch (e) {
 console.error('retryTasksfailed:', e);
 alert(e.message);
 }
}
function startInlineEditSchedule() {
 const container = document.getElementById('bq-schedule-val');
 if (!container) return;
 const queueId = batchQueuesState.currentQueueId;
 if (!queueId) return;
 apiFetch(`/api/batch-tasks/${queueId}`).then(r => r.json()).then(detail => {
 const queue = detail.queue;
 const isCron = queue.scheduleMode === 'cron';
 container.innerHTML = `<span class="bq-inline-edit-controls">
 <select id="bq-edit-schedule-mode" onchange="toggleInlineScheduleCron()">
 <option value="manual" ${!isCron ? 'selected' : ''}>${escapeHtml(_t('batchImportModal.scheduleModeManual'))}</option>
 <option value="cron" ${isCron ? 'selected' : ''}>${escapeHtml(_t('batchImportModal.scheduleModeCron'))}</option>
 </select>
 <input type="text" id="bq-edit-cron-expr" value="${escapeHtml(queue.cronExpr || '')}" placeholder="${_t('batchImportModal.cronExprPlaceholder', { interpolation: { escapeValue: false } })}" style="width:200px;${!isCron ? 'display:none;' : ''}" />
 </span>`;
 let schedCancelled = false;
 const sel = document.getElementById('bq-edit-schedule-mode');
 const cronInp = document.getElementById('bq-edit-cron-expr');
 if (sel) {
 sel.focus();
 sel.addEventListener('keydown', (e) => { if (e.key === 'Escape') { schedCancelled = true; cancelAllInlineEdits(); } });
 sel.addEventListener('change', () => {
 toggleInlineScheduleCron();
 if (sel.value !== 'cron' && !schedCancelled) saveInlineSchedule();
 });
 sel.addEventListener('blur', (e) => {
 setTimeout(() => {
 const active = document.activeElement;
 if (active && (active.id === 'bq-edit-cron-expr' || active.id === 'bq-edit-schedule-mode')) return;
 if (!schedCancelled) saveInlineSchedule();
 }, 100);
 });
 }
 if (cronInp) {
 cronInp.addEventListener('keydown', (e) => {
 if (e.key === 'Enter') { e.preventDefault(); cronInp.blur(); }
 if (e.key === 'Escape') { schedCancelled = true; cancelAllInlineEdits(); }
 });
 cronInp.addEventListener('blur', () => {
 setTimeout(() => {
 const active = document.activeElement;
 if (active && (active.id === 'bq-edit-cron-expr' || active.id === 'bq-edit-schedule-mode')) return;
 if (!schedCancelled) saveInlineSchedule();
 }, 100);
 });
 }
 });
}
function toggleInlineScheduleCron() {
 const modeSelect = document.getElementById('bq-edit-schedule-mode');
 const cronInput = document.getElementById('bq-edit-cron-expr');
 if (modeSelect && cronInput) {
 cronInput.style.display = modeSelect.value === 'cron' ? '' : 'none';
 if (modeSelect.value === 'cron') cronInput.focus();
 }
}
async function saveInlineSchedule() {
 if (_bqInlineSaving) return;
 _bqInlineSaving = true;
 const queueId = batchQueuesState.currentQueueId;
 if (!queueId) { _bqInlineSaving = false; return; }
 const modeSelect = document.getElementById('bq-edit-schedule-mode');
 const cronInput = document.getElementById('bq-edit-cron-expr');
 if (!modeSelect) { _bqInlineSaving = false; return; }
 const scheduleMode = modeSelect.value;
 const cronExpr = cronInput ? cronInput.value.trim() : '';
 if (scheduleMode === 'cron' && !cronExpr) {
 _bqInlineSaving = false;
 alert(_t('batchImportModal.cronExprRequired'));
 return;
 }
 if (scheduleMode === 'cron' && !/^\S+\s+\S+\s+\S+\s+\S+\s+\S+$/.test(cronExpr)) {
 _bqInlineSaving = false;
 alert(_t('batchImportModal.cronExprInvalid') || 'Invalid Cron expression format. Must have 5 fields (minute hour day month weekday), e.g.: 0 */2 * * *');
 return;
 }
 try {
 const response = await apiFetch(`/api/batch-tasks/${queueId}/schedule`, {
 method: 'PUT',
 headers: { 'Content-Type': 'application/json' },
 body: JSON.stringify({ scheduleMode, cronExpr }),
 });
 if (!response.ok) {
 const result = await response.json().catch(() => ({}));
 throw new Error(result.error || _t('batchQueueDetailModal.editScheduleError'));
 }
 _bqInlineSaving = false;
 showBatchQueueDetail(queueId);
 refreshBatchQueues();
 } catch (e) {
 _bqInlineSaving = false;
 console.error(e);
 alert(_t('batchQueueDetailModal.editScheduleError') + ': ' + e.message);
 }
}
window.showBatchImportModal = showBatchImportModal;
window.closeBatchImportModal = closeBatchImportModal;
window.createBatchQueue = createBatchQueue;
window.showBatchQueueDetail = showBatchQueueDetail;
window.startBatchQueue = startBatchQueue;
window.pauseBatchQueue = pauseBatchQueue;
window.rerunBatchQueue = rerunBatchQueue;
window.deleteBatchQueue = deleteBatchQueue;
window.closeBatchQueueDetailModal = closeBatchQueueDetailModal;
window.refreshBatchQueues = refreshBatchQueues;
window.viewBatchTaskConversation = viewBatchTaskConversation;
window.editBatchTaskFromElement = editBatchTaskFromElement;
window.cancelInlineTask = cancelInlineTask;
window.saveInlineTask = saveInlineTask;
window.filterBatchQueues = filterBatchQueues;
window.goBatchQueuesPage = goBatchQueuesPage;
window.changeBatchQueuesPageSize = changeBatchQueuesPageSize;
window.showAddBatchTaskModal = showAddBatchTaskModal;
window.closeAddBatchTaskModal = closeAddBatchTaskModal;
window.saveAddBatchTask = saveAddBatchTask;
window.deleteBatchTaskFromElement = deleteBatchTaskFromElement;
window.deleteBatchQueueFromList = deleteBatchQueueFromList;
window.handleBatchScheduleModeChange = handleBatchScheduleModeChange;
window.updateBatchQueueScheduleEnabled = updateBatchQueueScheduleEnabled;
window.cancelAllInlineEdits = cancelAllInlineEdits;
window.startInlineEditTitle = startInlineEditTitle;
window.saveInlineTitle = saveInlineTitle;
window.startInlineEditRole = startInlineEditRole;
window.saveInlineRole = saveInlineRole;
window.startInlineEditAgentMode = startInlineEditAgentMode;
window.saveInlineAgentMode = saveInlineAgentMode;
window.retryBatchTask = retryBatchTask;
window.startInlineEditSchedule = startInlineEditSchedule;
window.toggleInlineScheduleCron = toggleInlineScheduleCron;
window.saveInlineSchedule = saveInlineSchedule;
document.addEventListener('languagechange', function () {
 try {
 const tasksPage = document.getElementById('page-tasks');
 if (!tasksPage || !tasksPage.classList.contains('active')) {
 return;
 }
 if (document.getElementById('batch-queues-list')) {
 renderBatchQueues();
 }
 const detailModal = document.getElementById('batch-queue-detail-modal');
 if (
 detailModal &&
 detailModal.style.display === 'block' &&
 batchQueuesState.currentQueueId
 ) {
 showBatchQueueDetail(batchQueuesState.currentQueueId);
 }
 } catch (e) {
 console.warn('languagechange tasks refresh failed', e);
 }
});
