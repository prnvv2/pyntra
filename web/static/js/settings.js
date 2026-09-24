let currentConfig = null;
let allTools = [];
let toolStateMap = new Map();
function getToolKey(tool) {
 if (tool.is_external && tool.external_mcp) {
 return `${tool.external_mcp}::${tool.name}`;
 }
 return tool.name;
}
const getToolsPageSize = () => {
 const saved = localStorage.getItem('toolsPageSize');
 return saved ? parseInt(saved, 10) : 20;
};

let toolsPagination = {
 page: 1,
 pageSize: getToolsPageSize(),
 total: 0,
 totalPages: 0
};
function switchSettingsSection(section) {
 document.querySelectorAll('.settings-nav-item').forEach(item => {
 item.classList.remove('active');
 });
 const activeNavItem = document.querySelector(`.settings-nav-item[data-section="${section}"]`);
 if (activeNavItem) {
 activeNavItem.classList.add('active');
 }
 document.querySelectorAll('.settings-section-content').forEach(content => {
 content.classList.remove('active');
 });
 const activeContent = document.getElementById(`settings-section-${section}`);
 if (activeContent) {
 activeContent.classList.add('active');
 }
 if (section === 'terminal' && typeof initTerminal === 'function') {
 setTimeout(initTerminal, 0);
 }
}
async function openSettings() {
 if (typeof switchPage === 'function') {
 switchPage('settings');
 }
 toolStateMap.clear();
 await loadConfig(false);
 document.querySelectorAll('.form-group input').forEach(input => {
 input.classList.remove('error');
 });
 switchSettingsSection('basic');
}
function closeSettings() {
 if (typeof switchPage === 'function') {
 switchPage('chat');
 }
}
window.onclick = function(event) {
 const mcpModal = document.getElementById('mcp-detail-modal');
 
 if (event.target === mcpModal) {
 closeMCPDetail();
 }
}
async function loadConfig(loadTools = true) {
 try {
 const response = await apiFetch('/api/config');
 if (!response.ok) {
 throw new Error('Failed to fetch configuration');
 }
 
 currentConfig = await response.json();
 const providerEl = document.getElementById('openai-provider');
 if (providerEl) {
 const providerVal = currentConfig.openai.provider || 'openai';
 // Keep the configured provider selectable so saving round-trips it unchanged
 // (e.g. "ollama" from config.yaml is not one of the two built-in options).
 if (!Array.from(providerEl.options).some(o => o.value === providerVal)) {
 const opt = document.createElement('option');
 opt.value = providerVal;
 opt.textContent = providerVal + ' (from config)';
 providerEl.appendChild(opt);
 }
 providerEl.value = providerVal;
 }
 document.getElementById('openai-api-key').value = currentConfig.openai.api_key || '';
 document.getElementById('openai-base-url').value = currentConfig.openai.base_url || '';
 document.getElementById('openai-model').value = currentConfig.openai.model || '';
 const maxTokensEl = document.getElementById('openai-max-total-tokens');
 if (maxTokensEl) {
 maxTokensEl.value = currentConfig.openai.max_total_tokens || 120000;
 }
 document.getElementById('agent-max-iterations').value = currentConfig.agent.max_iterations || 30;

 const ma = currentConfig.multi_agent || {};
 const maEn = document.getElementById('multi-agent-enabled');
 if (maEn) maEn.checked = ma.enabled === true;
 const maPeLoop = document.getElementById('multi-agent-pe-loop');
 if (maPeLoop) {
 const v = ma.plan_execute_loop_max_iterations;
 maPeLoop.value = (v !== undefined && v !== null && !Number.isNaN(Number(v))) ? String(Number(v)) : '0';
 }
 const maMode = document.getElementById('multi-agent-default-mode');
 if (maMode) maMode.value = (ma.default_mode === 'multi') ? 'multi' : 'single';
 const knowledgeEnabledCheckbox = document.getElementById('knowledge-enabled');
 if (knowledgeEnabledCheckbox) {
 knowledgeEnabledCheckbox.checked = currentConfig.knowledge?.enabled !== false;
 }
 if (currentConfig.knowledge) {
 const knowledge = currentConfig.knowledge;
 const basePathInput = document.getElementById('knowledge-base-path');
 if (basePathInput) {
 basePathInput.value = knowledge.base_path || 'knowledge_base';
 }
 const embeddingProviderSelect = document.getElementById('knowledge-embedding-provider');
 if (embeddingProviderSelect) {
 embeddingProviderSelect.value = knowledge.embedding?.provider || 'openai';
 }
 
 const embeddingModelInput = document.getElementById('knowledge-embedding-model');
 if (embeddingModelInput) {
 embeddingModelInput.value = knowledge.embedding?.model || '';
 }
 
 const embeddingBaseUrlInput = document.getElementById('knowledge-embedding-base-url');
 if (embeddingBaseUrlInput) {
 embeddingBaseUrlInput.value = knowledge.embedding?.base_url || '';
 }
 
 const embeddingApiKeyInput = document.getElementById('knowledge-embedding-api-key');
 if (embeddingApiKeyInput) {
 embeddingApiKeyInput.value = knowledge.embedding?.api_key || '';
 }
 const retrievalTopKInput = document.getElementById('knowledge-retrieval-top-k');
 if (retrievalTopKInput) {
 retrievalTopKInput.value = knowledge.retrieval?.top_k || 5;
 }
 
 const retrievalThresholdInput = document.getElementById('knowledge-retrieval-similarity-threshold');
 if (retrievalThresholdInput) {
 retrievalThresholdInput.value = knowledge.retrieval?.similarity_threshold || 0.7;
 }
 
 const subIdxFilterInput = document.getElementById('knowledge-retrieval-sub-index-filter');
 if (subIdxFilterInput) {
 subIdxFilterInput.value = knowledge.retrieval?.sub_index_filter || '';
 }

 const post = knowledge.retrieval?.post_retrieve || {};
 const prefetchInput = document.getElementById('knowledge-post-retrieve-prefetch-top-k');
 if (prefetchInput) {
 prefetchInput.value = post.prefetch_top_k ?? 0;
 }
 const maxCharsInput = document.getElementById('knowledge-post-retrieve-max-chars');
 if (maxCharsInput) {
 maxCharsInput.value = post.max_context_chars ?? 0;
 }
 const maxTokInput = document.getElementById('knowledge-post-retrieve-max-tokens');
 if (maxTokInput) {
 maxTokInput.value = post.max_context_tokens ?? 0;
 }
 const indexing = knowledge.indexing || {};
 const chunkStrategySelect = document.getElementById('knowledge-indexing-chunk-strategy');
 if (chunkStrategySelect) {
 const v = (indexing.chunk_strategy || 'markdown_then_recursive').toLowerCase();
 chunkStrategySelect.value = v === 'recursive' ? 'recursive' : 'markdown_then_recursive';
 }
 const reqTimeoutInput = document.getElementById('knowledge-indexing-request-timeout');
 if (reqTimeoutInput) {
 reqTimeoutInput.value = indexing.request_timeout_seconds ?? 120;
 }
 const batchSizeInput = document.getElementById('knowledge-indexing-batch-size');
 if (batchSizeInput) {
 batchSizeInput.value = indexing.batch_size ?? 64;
 }
 const preferFileCb = document.getElementById('knowledge-indexing-prefer-source-file');
 if (preferFileCb) {
 preferFileCb.checked = indexing.prefer_source_file === true;
 }
 const subIdxInput = document.getElementById('knowledge-indexing-sub-indexes');
 if (subIdxInput) {
 const arr = indexing.sub_indexes;
 subIdxInput.value = Array.isArray(arr) ? arr.join(', ') : (typeof arr === 'string' ? arr : '');
 }
 const chunkSizeInput = document.getElementById('knowledge-indexing-chunk-size');
 if (chunkSizeInput) {
 chunkSizeInput.value = indexing.chunk_size || 512;
 }

 const chunkOverlapInput = document.getElementById('knowledge-indexing-chunk-overlap');
 if (chunkOverlapInput) {
 chunkOverlapInput.value = indexing.chunk_overlap ?? 50;
 }

 const maxChunksPerItemInput = document.getElementById('knowledge-indexing-max-chunks-per-item');
 if (maxChunksPerItemInput) {
 maxChunksPerItemInput.value = indexing.max_chunks_per_item ?? 0;
 }

 const maxRpmInput = document.getElementById('knowledge-indexing-max-rpm');
 if (maxRpmInput) {
 maxRpmInput.value = indexing.max_rpm ?? 0;
 }

 const rateLimitDelayInput = document.getElementById('knowledge-indexing-rate-limit-delay-ms');
 if (rateLimitDelayInput) {
 rateLimitDelayInput.value = indexing.rate_limit_delay_ms ?? 300;
 }

 const maxRetriesInput = document.getElementById('knowledge-indexing-max-retries');
 if (maxRetriesInput) {
 maxRetriesInput.value = indexing.max_retries ?? 3;
 }

 const retryDelayInput = document.getElementById('knowledge-indexing-retry-delay-ms');
 if (retryDelayInput) {
 retryDelayInput.value = indexing.retry_delay_ms ?? 1000;
 }
 }
 if (loadTools) {
 const savedPageSize = getToolsPageSize();
 toolsPagination.pageSize = savedPageSize;
 toolsSearchKeyword = '';
 await loadToolsList(1, '');
 }
 } catch (error) {
 console.error('loadconfigfailed:', error);
 const baseMsg = (typeof window !== 'undefined' && typeof window.t === 'function')
 ? window.t('settings.apply.loadFailed')
 : 'Failed to load configuration';
 alert(baseMsg + ': ' + error.message);
 }
}
let toolsSearchKeyword = '';
let toolsStatusFilter = '';
async function loadToolsList(page = 1, searchKeyword = '') {
 if (window.i18nReady) await window.i18nReady;
 const toolsList = document.getElementById('tools-list');
 if (toolsList) {
 toolsList.innerHTML = '<div class="tools-list-items"><div class="loading" style="padding: 20px; text-align: center; color: var(--text-muted);">' + (typeof window.t === 'function' ? window.t('mcp.loadingTools') : 'Loading tools...') + '</div></div>';
 }
 
 try {
 saveCurrentPageToolStates();
 
 const pageSize = toolsPagination.pageSize;
 let url = `/api/config/tools?page=${page}&page_size=${pageSize}`;
 if (searchKeyword) {
 url += `&search=${encodeURIComponent(searchKeyword)}`;
 }
 if (toolsStatusFilter !== '') {
 url += `&enabled=${toolsStatusFilter}`;
 }
 const controller = new AbortController();
 const timeoutId = setTimeout(() => controller.abort(), 10000);
 
 const response = await apiFetch(url, {
 signal: controller.signal
 });
 clearTimeout(timeoutId);
 
 if (!response.ok) {
 throw new Error('Failed to fetch tool list');
 }
 
 const result = await response.json();
 allTools = result.tools || [];
 toolsPagination = {
 page: result.page || page,
 pageSize: result.page_size || pageSize,
 total: result.total || 0,
 totalPages: result.total_pages || 1
 };
 allTools.forEach(tool => {
 const toolKey = getToolKey(tool);
 if (!toolStateMap.has(toolKey)) {
 toolStateMap.set(toolKey, {
 enabled: tool.enabled,
 is_external: tool.is_external || false,
 external_mcp: tool.external_mcp || '',
 name: tool.name // savetoolname
 });
 }
 });
 
 renderToolsList();
 renderToolsPagination();
 } catch (error) {
 console.error('Failed to load tool list:', error);
 if (toolsList) {
 const isTimeout = error.name === 'AbortError' || error.message.includes('timeout');
 const errorMsg = isTimeout 
 ? (typeof window.t === 'function' ? window.t('mcp.loadToolsTimeout') : "Tools load timeout. External MCP may be slow. Click Refresh to retry or check connection.")
 : (typeof window.t === 'function' ? window.t('mcp.loadToolsFailed') : 'Failed to load tools') + ': ' + escapeHtml(error.message);
 toolsList.innerHTML = `<div class="error" style="padding: 20px; text-align: center;">${errorMsg}</div>`;
 }
 }
}
function saveCurrentPageToolStates() {
 document.querySelectorAll('#tools-list .tool-item').forEach(item => {
 const checkbox = item.querySelector('input[type="checkbox"]');
 const toolKey = item.dataset.toolKey; // 
 const toolName = item.dataset.toolName;
 const isExternal = item.dataset.isExternal === 'true';
 const externalMcp = item.dataset.externalMcp || '';
 if (toolKey && checkbox) {
 toolStateMap.set(toolKey, {
 enabled: checkbox.checked,
 is_external: isExternal,
 external_mcp: externalMcp,
 name: toolName // savetoolname
 });
 }
 });
}
function searchTools() {
 const searchInput = document.getElementById('tools-search');
 const keyword = searchInput ? searchInput.value.trim() : '';
 toolsSearchKeyword = keyword;
 loadToolsList(1, keyword);
}
function clearSearch() {
 const searchInput = document.getElementById('tools-search');
 if (searchInput) {
 searchInput.value = '';
 }
 toolsSearchKeyword = '';
 loadToolsList(1, '');
}
function handleSearchKeyPress(event) {
 if (event.key === 'Enter') {
 searchTools();
 }
}
function filterToolsByStatus(status) {
 toolsStatusFilter = status;
 document.querySelectorAll('.tools-status-filter .btn-filter').forEach(btn => {
 btn.classList.toggle('active', btn.dataset.filter === status);
 });
 loadToolsList(1, toolsSearchKeyword);
}
function renderToolsList() {
 const toolsList = document.getElementById('tools-list');
 if (!toolsList) return;
 const oldPagination = toolsList.querySelector('.tools-pagination');
 if (oldPagination) {
 oldPagination.remove();
 }
 let listContainer = toolsList.querySelector('.tools-list-items');
 if (!listContainer) {
 listContainer = document.createElement('div');
 listContainer.className = 'tools-list-items';
 toolsList.appendChild(listContainer);
 }
 listContainer.innerHTML = '';
 
 if (allTools.length === 0) {
 listContainer.innerHTML = '<div class="empty">' + (typeof window.t === 'function' ? window.t('mcp.noTools') : 'No tools') + '</div>';
 if (!toolsList.contains(listContainer)) {
 toolsList.appendChild(listContainer);
 }
 updateToolsStats();
 return;
 }
 
 allTools.forEach(tool => {
 const toolKey = getToolKey(tool); // generate
 const toolItem = document.createElement('div');
 toolItem.className = 'tool-item';
 toolItem.dataset.toolKey = toolKey; // save
 toolItem.dataset.toolName = tool.name; // savetoolname
 toolItem.dataset.isExternal = tool.is_external ? 'true' : 'false';
 toolItem.dataset.externalMcp = tool.external_mcp || '';
 const toolState = toolStateMap.get(toolKey) || {
 enabled: tool.enabled,
 is_external: tool.is_external || false,
 external_mcp: tool.external_mcp || ''
 };
 let externalBadge = '';
 if (toolState.is_external || tool.is_external) {
 const externalMcpName = toolState.external_mcp || tool.external_mcp || '';
 const badgeText = externalMcpName ? (typeof window.t === 'function' ? window.t('mcp.externalFrom', { name: escapeHtml(externalMcpName) }) : ` (${escapeHtml(externalMcpName)})`) : (typeof window.t === 'function' ? window.t('mcp.externalBadge') : 'External');
 const badgeTitle = externalMcpName ? (typeof window.t === 'function' ? window.t('mcp.externalToolFrom', { name: escapeHtml(externalMcpName) }) : `MCPtool - :${escapeHtml(externalMcpName)}`) : (typeof window.t === 'function' ? window.t('mcp.externalBadge') : 'External');
 externalBadge = `<span class="external-tool-badge" title="${badgeTitle}">${badgeText}</span>`;
 }
 const checkboxId = `tool-${escapeHtml(toolKey).replace(/::/g, '--')}`;
 
 toolItem.innerHTML = `
 <input type="checkbox" id="${checkboxId}" ${toolState.enabled ? 'checked' : ''} ${toolState.is_external || tool.is_external ? 'data-external="true"' : ''} onchange="handleToolCheckboxChange('${escapeHtml(toolKey)}', this.checked)" />
 <div class="tool-item-info">
 <div class="tool-item-name">
 ${escapeHtml(tool.name)}
 ${externalBadge}
 </div>
 <div class="tool-item-desc">${escapeHtml(tool.description || (typeof window.t === 'function' ? window.t('mcp.noDescription') : 'No description'))}</div>
 </div>
 `;
 listContainer.appendChild(toolItem);
 });
 
 if (!toolsList.contains(listContainer)) {
 toolsList.appendChild(listContainer);
 }
 updateToolsStats();
}
function renderToolsPagination() {
 const toolsList = document.getElementById('tools-list');
 if (!toolsList) return;
 const oldPagination = toolsList.querySelector('.tools-pagination');
 if (oldPagination) {
 oldPagination.remove();
 }
 if (toolsPagination.totalPages <= 1) {
 return;
 }
 
 const pagination = document.createElement('div');
 pagination.className = 'tools-pagination';
 
 const { page, totalPages, total } = toolsPagination;
 const startItem = (page - 1) * toolsPagination.pageSize + 1;
 const endItem = Math.min(page * toolsPagination.pageSize, total);
 
 const savedPageSize = getToolsPageSize();
 const t = typeof window.t === 'function' ? window.t : (k) => k;
 const paginationT = (key, opts) => {
 if (typeof window.t === 'function') return window.t(key, opts);
 if (key === 'mcp.paginationInfo' && opts) return ` ${opts.start}-${opts.end} / ${opts.total} tool`;
 if (key === 'mcp.pageInfo' && opts) return ` ${opts.page} / ${opts.total} `;
 return key;
 };
 pagination.innerHTML = `
 <div class="pagination-info">
 ${paginationT('mcp.paginationInfo', { start: startItem, end: endItem, total: total })}${toolsSearchKeyword ? ` (${t('common.search')}: "${escapeHtml(toolsSearchKeyword)}")` : ''}
 </div>
 <div class="pagination-page-size">
 <label for="tools-page-size-pagination">${t('mcp.perPage')}</label>
 <select id="tools-page-size-pagination" onchange="changeToolsPageSize()">
 <option value="10" ${savedPageSize === 10 ? 'selected' : ''}>10</option>
 <option value="20" ${savedPageSize === 20 ? 'selected' : ''}>20</option>
 <option value="50" ${savedPageSize === 50 ? 'selected' : ''}>50</option>
 <option value="100" ${savedPageSize === 100 ? 'selected' : ''}>100</option>
 </select>
 </div>
 <div class="pagination-controls">
 <button class="btn-secondary" onclick="loadToolsList(1, '${escapeHtml(toolsSearchKeyword)}')" ${page === 1 ? 'disabled' : ''}>${t('mcp.firstPage')}</button>
 <button class="btn-secondary" onclick="loadToolsList(${page - 1}, '${escapeHtml(toolsSearchKeyword)}')" ${page === 1 ? 'disabled' : ''}>${t('mcp.prevPage')}</button>
 <span class="pagination-page">${paginationT('mcp.pageInfo', { page: page, total: totalPages })}</span>
 <button class="btn-secondary" onclick="loadToolsList(${page + 1}, '${escapeHtml(toolsSearchKeyword)}')" ${page === totalPages ? 'disabled' : ''}>${t('mcp.nextPage')}</button>
 <button class="btn-secondary" onclick="loadToolsList(${totalPages}, '${escapeHtml(toolsSearchKeyword)}')" ${page === totalPages ? 'disabled' : ''}>${t('mcp.lastPage')}</button>
 </div>
 `;
 
 toolsList.appendChild(pagination);
}
function handleToolCheckboxChange(toolKey, enabled) {
 const toolItem = document.querySelector(`.tool-item[data-tool-key="${toolKey}"]`);
 if (toolItem) {
 const toolName = toolItem.dataset.toolName;
 const isExternal = toolItem.dataset.isExternal === 'true';
 const externalMcp = toolItem.dataset.externalMcp || '';
 toolStateMap.set(toolKey, {
 enabled: enabled,
 is_external: isExternal,
 external_mcp: externalMcp,
 name: toolName // savetoolname
 });
 }
 updateToolsStats();
}
function selectAllTools() {
 document.querySelectorAll('#tools-list input[type="checkbox"]').forEach(checkbox => {
 checkbox.checked = true;
 const toolItem = checkbox.closest('.tool-item');
 if (toolItem) {
 const toolKey = toolItem.dataset.toolKey;
 const toolName = toolItem.dataset.toolName;
 const isExternal = toolItem.dataset.isExternal === 'true';
 const externalMcp = toolItem.dataset.externalMcp || '';
 if (toolKey) {
 toolStateMap.set(toolKey, {
 enabled: true,
 is_external: isExternal,
 external_mcp: externalMcp,
 name: toolName // savetoolname
 });
 }
 }
 });
 updateToolsStats();
}
function deselectAllTools() {
 document.querySelectorAll('#tools-list input[type="checkbox"]').forEach(checkbox => {
 checkbox.checked = false;
 const toolItem = checkbox.closest('.tool-item');
 if (toolItem) {
 const toolKey = toolItem.dataset.toolKey;
 const toolName = toolItem.dataset.toolName;
 const isExternal = toolItem.dataset.isExternal === 'true';
 const externalMcp = toolItem.dataset.externalMcp || '';
 if (toolKey) {
 toolStateMap.set(toolKey, {
 enabled: false,
 is_external: isExternal,
 external_mcp: externalMcp,
 name: toolName // savetoolname
 });
 }
 }
 });
 updateToolsStats();
}
async function changeToolsPageSize() {
 const pageSizeSelect = document.getElementById('tools-page-size') || document.getElementById('tools-page-size-pagination');
 if (!pageSizeSelect) return;
 
 const newPageSize = parseInt(pageSizeSelect.value, 10);
 if (isNaN(newPageSize) || newPageSize < 1) {
 return;
 }
 localStorage.setItem('toolsPageSize', newPageSize.toString());
 toolsPagination.pageSize = newPageSize;
 const otherSelect = document.getElementById('tools-page-size') || document.getElementById('tools-page-size-pagination');
 if (otherSelect && otherSelect !== pageSizeSelect) {
 otherSelect.value = newPageSize;
 }
 await loadToolsList(1, toolsSearchKeyword);
}
async function updateToolsStats() {
 const statsEl = document.getElementById('tools-stats');
 if (!statsEl) return;
 saveCurrentPageToolStates();
 const currentPageEnabled = Array.from(document.querySelectorAll('#tools-list input[type="checkbox"]:checked')).length;
 const currentPageTotal = document.querySelectorAll('#tools-list input[type="checkbox"]').length;
 let totalEnabled = 0;
 let totalTools = toolsPagination.total || 0;
 
 try {
 if (toolsSearchKeyword) {
 totalTools = allTools.length;
 totalEnabled = allTools.filter(tool => {
 const toolKey = getToolKey(tool);
 const savedState = toolStateMap.get(toolKey);
 if (savedState !== undefined) {
 return savedState.enabled;
 }
 const checkboxId = `tool-${toolKey.replace(/::/g, '--')}`;
 const checkbox = document.getElementById(checkboxId);
 return checkbox ? checkbox.checked : tool.enabled;
 }).length;
 } else {
 const localStateMap = new Map();
 allTools.forEach(tool => {
 const toolKey = getToolKey(tool);
 const savedState = toolStateMap.get(toolKey);
 if (savedState !== undefined) {
 localStateMap.set(toolKey, savedState.enabled);
 } else {
 const checkboxId = `tool-${toolKey.replace(/::/g, '--')}`;
 const checkbox = document.getElementById(checkboxId);
 if (checkbox) {
 localStateMap.set(toolKey, checkbox.checked);
 } else {
 localStateMap.set(toolKey, tool.enabled);
 }
 }
 });
 if (totalTools > allTools.length) {
 let page = 1;
 let hasMore = true;
 const pageSize = 100; // page
 
 while (hasMore && page <= 10) { // 10,
 const url = `/api/config/tools?page=${page}&page_size=${pageSize}`;
 const pageResponse = await apiFetch(url);
 if (!pageResponse.ok) break;
 
 const pageResult = await pageResponse.json();
 pageResult.tools.forEach(tool => {
 const toolKey = getToolKey(tool);
 if (!localStateMap.has(toolKey)) {
 const savedState = toolStateMap.get(toolKey);
 localStateMap.set(toolKey, savedState ? savedState.enabled : tool.enabled);
 }
 });
 
 if (page >= pageResult.total_pages) {
 hasMore = false;
 } else {
 page++;
 }
 }
 }
 totalEnabled = Array.from(localStateMap.values()).filter(enabled => enabled).length;
 }
 } catch (error) {
 console.warn('fetchtoolfailed,current', error);
 totalTools = totalTools || currentPageTotal;
 totalEnabled = currentPageEnabled;
 }
 
 const tStats = typeof window.t === 'function' ? window.t : (k) => k;
 statsEl.innerHTML = `
 <span title="${tStats('mcp.currentPageEnabled')}">${tStats('mcp.currentPageEnabled')}: <strong>${currentPageEnabled}</strong> / ${currentPageTotal}</span>
 <span title="${tStats('mcp.totalEnabled')}">${tStats('mcp.totalEnabled')}: <strong>${totalEnabled}</strong> / ${totalTools}</span>
 `;
}
function filterTools() {
}
async function applySettings() {
 try {
 document.querySelectorAll('.form-group input').forEach(input => {
 input.classList.remove('error');
 });
 const provider = document.getElementById('openai-provider')?.value || 'openai';
 const apiKey = document.getElementById('openai-api-key').value.trim();
 const baseUrl = document.getElementById('openai-base-url').value.trim();
 const model = document.getElementById('openai-model').value.trim();
 
 let hasError = false;
 
 if (!apiKey) {
 document.getElementById('openai-api-key').classList.add('error');
 hasError = true;
 }
 
 if (!baseUrl) {
 document.getElementById('openai-base-url').classList.add('error');
 hasError = true;
 }
 
 if (!model) {
 document.getElementById('openai-model').classList.add('error');
 hasError = true;
 }
 
 if (hasError) {
 const msg = (typeof window !== 'undefined' && typeof window.t === 'function')
 ? window.t('settings.apply.fillRequired')
 : 'Please fill in all required fields (marked with *)';
 alert(msg);
 return;
 }
 const knowledgeEnabledCheckbox = document.getElementById('knowledge-enabled');
 const knowledgeEnabled = knowledgeEnabledCheckbox ? knowledgeEnabledCheckbox.checked : true;
 const knowledgeConfig = {
 enabled: knowledgeEnabled,
 base_path: document.getElementById('knowledge-base-path')?.value.trim() || 'knowledge_base',
 embedding: {
 provider: document.getElementById('knowledge-embedding-provider')?.value || 'openai',
 model: document.getElementById('knowledge-embedding-model')?.value.trim() || '',
 base_url: document.getElementById('knowledge-embedding-base-url')?.value.trim() || '',
 api_key: document.getElementById('knowledge-embedding-api-key')?.value.trim() || ''
 },
 retrieval: {
 top_k: parseInt(document.getElementById('knowledge-retrieval-top-k')?.value) || 5,
 similarity_threshold: (() => {
 const val = parseFloat(document.getElementById('knowledge-retrieval-similarity-threshold')?.value);
 return isNaN(val) ? 0.7 : val;
 })(),
 sub_index_filter: document.getElementById('knowledge-retrieval-sub-index-filter')?.value?.trim() || '',
 post_retrieve: {
 prefetch_top_k: parseInt(document.getElementById('knowledge-post-retrieve-prefetch-top-k')?.value, 10) || 0,
 max_context_chars: parseInt(document.getElementById('knowledge-post-retrieve-max-chars')?.value, 10) || 0,
 max_context_tokens: parseInt(document.getElementById('knowledge-post-retrieve-max-tokens')?.value, 10) || 0
 }
 },
 indexing: (() => {
 const subRaw = document.getElementById("knowledge-indexing-sub-indexes")?.value?.trim() || "";
 const sub_indexes = subRaw
 ? subRaw.split(/[,,]/).map(s => s.trim()).filter(Boolean)
 : [];
 return {
 chunk_strategy: document.getElementById("knowledge-indexing-chunk-strategy")?.value || "markdown_then_recursive",
 request_timeout_seconds: parseInt(document.getElementById("knowledge-indexing-request-timeout")?.value, 10) || 0,
 batch_size: parseInt(document.getElementById("knowledge-indexing-batch-size")?.value, 10) || 0,
 prefer_source_file: document.getElementById("knowledge-indexing-prefer-source-file")?.checked === true,
 sub_indexes,
 chunk_size: parseInt(document.getElementById("knowledge-indexing-chunk-size")?.value) || 512,
 chunk_overlap: parseInt(document.getElementById("knowledge-indexing-chunk-overlap")?.value) ?? 50,
 max_chunks_per_item: parseInt(document.getElementById("knowledge-indexing-max-chunks-per-item")?.value) ?? 0,
 max_rpm: parseInt(document.getElementById("knowledge-indexing-max-rpm")?.value) ?? 0,
 rate_limit_delay_ms: parseInt(document.getElementById("knowledge-indexing-rate-limit-delay-ms")?.value) ?? 300,
 max_retries: parseInt(document.getElementById("knowledge-indexing-max-retries")?.value) ?? 3,
 retry_delay_ms: parseInt(document.getElementById("knowledge-indexing-retry-delay-ms")?.value) ?? 1000
 };
 })()
 };
 
 const config = {
 openai: {
 provider: provider,
 api_key: apiKey,
 base_url: baseUrl,
 model: model,
 max_total_tokens: parseInt(document.getElementById('openai-max-total-tokens')?.value) || 120000
 },
 agent: {
 max_iterations: parseInt(document.getElementById('agent-max-iterations').value) || 30
 },
 multi_agent: (function () {
 const peRaw = document.getElementById('multi-agent-pe-loop')?.value;
 const peParsed = parseInt(peRaw, 10);
 const peLoop = Number.isNaN(peParsed) ? 0 : Math.max(0, peParsed);
 return {
 enabled: document.getElementById('multi-agent-enabled')?.checked === true,
 default_mode: document.getElementById('multi-agent-default-mode')?.value === 'multi' ? 'multi' : 'single',
 batch_use_multi_agent: false,
 plan_execute_loop_max_iterations: peLoop
 };
 })(),
 knowledge: knowledgeConfig,
 tools: []
 };
 saveCurrentPageToolStates();
 try {
 const allToolsMap = new Map();
 let page = 1;
 let hasMore = true;
 const pageSize = 100; // page
 while (hasMore) {
 const url = `/api/config/tools?page=${page}&page_size=${pageSize}`;
 
 const pageResponse = await apiFetch(url);
 if (!pageResponse.ok) {
 throw new Error('Failed to fetch tool list');
 }
 
 const pageResult = await pageResponse.json();
 pageResult.tools.forEach(tool => {
 const toolKey = getToolKey(tool);
 const savedState = toolStateMap.get(toolKey);
 allToolsMap.set(toolKey, {
 name: tool.name,
 enabled: savedState ? savedState.enabled : tool.enabled,
 is_external: savedState ? savedState.is_external : (tool.is_external || false),
 external_mcp: savedState ? savedState.external_mcp : (tool.external_mcp || '')
 });
 });
 if (page >= pageResult.total_pages) {
 hasMore = false;
 } else {
 page++;
 }
 }
 allToolsMap.forEach((tool, toolKey) => {
 config.tools.push({
 name: tool.name,
 enabled: tool.enabled,
 is_external: tool.is_external,
 external_mcp: tool.external_mcp
 });
 });
 } catch (error) {
 console.warn('fetchtoolfailed,status', error);
 toolStateMap.forEach((toolData, toolKey) => {
 const toolName = toolData.name || toolKey.split('::').pop();
 config.tools.push({
 name: toolName,
 enabled: toolData.enabled,
 is_external: toolData.is_external,
 external_mcp: toolData.external_mcp
 });
 });
 }
 const updateResponse = await apiFetch('/api/config', {
 method: 'PUT',
 headers: {
 'Content-Type': 'application/json'
 },
 body: JSON.stringify(config)
 });
 
 if (!updateResponse.ok) {
 const error = await updateResponse.json();
 const fallback = (typeof window !== 'undefined' && typeof window.t === 'function')
 ? window.t('settings.apply.applyFailed')
 : 'Failed to apply configuration';
 throw new Error(error.error || fallback);
 }
 const applyResponse = await apiFetch('/api/config/apply', {
 method: 'POST'
 });
 
 if (!applyResponse.ok) {
 const error = await applyResponse.json();
 const fallback = (typeof window !== 'undefined' && typeof window.t === 'function')
 ? window.t('settings.apply.applyFailed')
 : 'Failed to apply configuration';
 throw new Error(error.error || fallback);
 }
 
 const successMsg = (typeof window !== 'undefined' && typeof window.t === 'function')
 ? window.t('settings.apply.applySuccess')
 : 'Configuration applied successfully!';
 alert(successMsg);
 try {
 if (typeof initChatAgentModeFromConfig === 'function') {
 await initChatAgentModeFromConfig();
 }
 } catch (e) {
 console.warn('initChatAgentModeFromConfig after settings', e);
 }
 closeSettings();
 } catch (error) {
 console.error('applyconfigfailed:', error);
 const baseMsg = (typeof window !== 'undefined' && typeof window.t === 'function')
 ? window.t('settings.apply.applyFailed')
 : 'Failed to apply configuration';
 alert(baseMsg + ': ' + error.message);
 }
}
async function testOpenAIConnection() {
 const btn = document.getElementById('test-openai-btn');
 const resultEl = document.getElementById('test-openai-result');

 const provider = document.getElementById('openai-provider')?.value || 'openai';
 const baseUrl = document.getElementById('openai-base-url').value.trim();
 const apiKey = document.getElementById('openai-api-key').value.trim();
 const model = document.getElementById('openai-model').value.trim();

 if (!apiKey || !model) {
 resultEl.style.color = 'var(--danger-color, #e53e3e)';
 resultEl.textContent = typeof window.t === 'function' ? window.t('settingsBasic.testFillRequired') : 'Please fill in API Key and Model first';
 return;
 }

 btn.style.pointerEvents = 'none';
 btn.style.opacity = '0.5';
 resultEl.style.color = 'var(--text-muted, #888)';
 resultEl.textContent = typeof window.t === 'function' ? window.t('settingsBasic.testing') : 'Testing connection...';

 try {
 const response = await apiFetch('/api/config/test-openai', {
 method: 'POST',
 headers: { 'Content-Type': 'application/json' },
 body: JSON.stringify({
 provider: provider,
 base_url: baseUrl,
 api_key: apiKey,
 model: model
 })
 });

 const result = await response.json();

 if (result.success) {
 resultEl.style.color = 'var(--success-color, #38a169)';
 const latency = result.latency_ms ? ` (${result.latency_ms}ms)` : '';
 const modelInfo = result.model ? ` [${result.model}]` : '';
 resultEl.textContent = (typeof window.t === 'function' ? window.t('settingsBasic.testSuccess') : 'Connection successful') + modelInfo + latency;
 } else {
 resultEl.style.color = 'var(--danger-color, #e53e3e)';
 resultEl.textContent = (typeof window.t === 'function' ? window.t('settingsBasic.testFailed') : 'Connection failed') + ': ' + (result.error || 'Unknown error');
 }
 } catch (error) {
 resultEl.style.color = 'var(--danger-color, #e53e3e)';
 resultEl.textContent = (typeof window.t === 'function' ? window.t('settingsBasic.testError') : 'Test error') + ': ' + error.message;
 } finally {
 btn.style.pointerEvents = '';
 btn.style.opacity = '';
 }
}
async function saveToolsConfig() {
 try {
 saveCurrentPageToolStates();
 const response = await apiFetch('/api/config');
 if (!response.ok) {
 throw new Error('Failed to fetch configuration');
 }
 
 const currentConfig = await response.json();
 const config = {
 openai: currentConfig.openai || {},
 agent: currentConfig.agent || {},
 tools: []
 };
 try {
 const allToolsMap = new Map();
 let page = 1;
 let hasMore = true;
 const pageSize = 100;
 while (hasMore) {
 const url = `/api/config/tools?page=${page}&page_size=${pageSize}`;
 
 const pageResponse = await apiFetch(url);
 if (!pageResponse.ok) {
 throw new Error('Failed to fetch tool list');
 }
 
 const pageResult = await pageResponse.json();
 pageResult.tools.forEach(tool => {
 const toolKey = getToolKey(tool);
 const savedState = toolStateMap.get(toolKey);
 allToolsMap.set(toolKey, {
 name: tool.name,
 enabled: savedState ? savedState.enabled : tool.enabled,
 is_external: savedState ? savedState.is_external : (tool.is_external || false),
 external_mcp: savedState ? savedState.external_mcp : (tool.external_mcp || '')
 });
 });
 if (page >= pageResult.total_pages) {
 hasMore = false;
 } else {
 page++;
 }
 }
 allToolsMap.forEach((tool, toolKey) => {
 config.tools.push({
 name: tool.name,
 enabled: tool.enabled,
 is_external: tool.is_external,
 external_mcp: tool.external_mcp
 });
 });
 } catch (error) {
 console.warn('fetchtoolfailed,status', error);
 toolStateMap.forEach((toolData, toolKey) => {
 const toolName = toolData.name || toolKey.split('::').pop();
 config.tools.push({
 name: toolName,
 enabled: toolData.enabled,
 is_external: toolData.is_external,
 external_mcp: toolData.external_mcp
 });
 });
 }
 const updateResponse = await apiFetch('/api/config', {
 method: 'PUT',
 headers: {
 'Content-Type': 'application/json'
 },
 body: JSON.stringify(config)
 });
 
 if (!updateResponse.ok) {
 const error = await updateResponse.json();
 throw new Error(error.error || 'updateconfigfailed');
 }
 const applyResponse = await apiFetch('/api/config/apply', {
 method: 'POST'
 });
 
 if (!applyResponse.ok) {
 const error = await applyResponse.json();
 throw new Error(error.error || 'applyconfigfailed');
 }
 
 alert(typeof window.t === 'function' ? window.t('mcp.toolsConfigSaved') : 'Tool configuration saved!');
 if (typeof loadToolsList === 'function') {
 await loadToolsList(toolsPagination.page, toolsSearchKeyword);
 }
 } catch (error) {
 console.error('savetoolconfigfailed:', error);
 alert((typeof window.t === 'function' ? window.t('mcp.saveToolsConfigFailed') : 'Failed to save tool config') + ': ' + error.message);
 }
}

function resetPasswordForm() {
 const currentInput = document.getElementById('auth-current-password');
 const newInput = document.getElementById('auth-new-password');
 const confirmInput = document.getElementById('auth-confirm-password');

 [currentInput, newInput, confirmInput].forEach(input => {
 if (input) {
 input.value = '';
 input.classList.remove('error');
 }
 });
}

async function changePassword() {
 const currentInput = document.getElementById('auth-current-password');
 const newInput = document.getElementById('auth-new-password');
 const confirmInput = document.getElementById('auth-confirm-password');
 const submitBtn = document.querySelector('.change-password-submit');

 [currentInput, newInput, confirmInput].forEach(input => input && input.classList.remove('error'));

 const currentPassword = currentInput?.value.trim() || '';
 const newPassword = newInput?.value.trim() || '';
 const confirmPassword = confirmInput?.value.trim() || '';

 let hasError = false;

 if (!currentPassword) {
 currentInput?.classList.add('error');
 hasError = true;
 }

 if (!newPassword || newPassword.length < 8) {
 newInput?.classList.add('error');
 hasError = true;
 }

 if (newPassword !== confirmPassword) {
 confirmInput?.classList.add('error');
 hasError = true;
 }

 if (hasError) {
 alert(typeof window.t === 'function' ? window.t('settings.security.fillPasswordHint') : 'Fill current and new password correctly. New password at least 8 characters, must match twice.');
 return;
 }

 if (submitBtn) {
 submitBtn.disabled = true;
 }

 try {
 const response = await apiFetch('/api/auth/change-password', {
 method: 'POST',
 headers: {
 'Content-Type': 'application/json'
 },
 body: JSON.stringify({
 oldPassword: currentPassword,
 newPassword: newPassword
 })
 });

 const result = await response.json().catch(() => ({}));
 if (!response.ok) {
 throw new Error(result.error || 'passwordfailed');
 }

 const pwdMsg = typeof window.t === 'function' ? window.t('settings.security.passwordUpdated') : 'Password updated. Please sign in again with new password.';
 alert(pwdMsg);
 resetPasswordForm();
 handleUnauthorized({ message: pwdMsg, silent: false });
 closeSettings();
 } catch (error) {
 console.error('passwordfailed:', error);
 alert((typeof window.t === 'function' ? window.t('settings.security.changePasswordFailed') : 'Failed to change password') + ': ' + error.message);
 } finally {
 if (submitBtn) {
 submitBtn.disabled = false;
 }
 }
}

let currentEditingMCPName = null;
async function fetchExternalMCPs() {
 const response = await apiFetch('/api/external-mcp');
 if (!response.ok) throw new Error('fetchMCPfailed');
 return response.json();
}
async function loadExternalMCPs() {
 try {
 if (window.i18nReady) await window.i18nReady;
 const data = await fetchExternalMCPs();
 renderExternalMCPList(data.servers || {});
 renderExternalMCPStats(data.stats || {});
 } catch (error) {
 console.error('loadMCPfailed:', error);
 const list = document.getElementById('external-mcp-list');
 if (list) {
 const errT = typeof window.t === 'function' ? window.t : (k) => k;
 list.innerHTML = `<div class="error">${escapeHtml(errT('mcp.loadExternalMCPFailed'))}: ${escapeHtml(error.message)}</div>`;
 }
 }
}
async function pollExternalMCPToolCount(name, maxAttempts = 10) {
 const pollIntervalMs = 1000;
 for (let attempt = 0; attempt < maxAttempts; attempt++) {
 await new Promise(r => setTimeout(r, pollIntervalMs));
 try {
 const data = await fetchExternalMCPs();
 renderExternalMCPList(data.servers || {});
 renderExternalMCPStats(data.stats || {});
 if (name != null) {
 const server = data.servers && data.servers[name];
 if (server && server.tool_count > 0) break;
 }
 } catch (e) {
 console.warn('toolfailed:', e);
 }
 }
 if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
 window.refreshMentionTools();
 }
}
function renderExternalMCPList(servers) {
 const list = document.getElementById('external-mcp-list');
 if (!list) return;
 
 if (Object.keys(servers).length === 0) {
 const emptyT = typeof window.t === 'function' ? window.t : (k) => k;
 list.innerHTML = '<div class="empty">' + emptyT('mcp.noExternalMCP') + '<br><span style="font-size: 0.875rem; margin-top: 8px; display: block;">' + emptyT('mcp.clickToAddExternal') + '</span></div>';
 return;
 }
 
 let html = '<div class="external-mcp-items">';
 for (const [name, server] of Object.entries(servers)) {
 const status = server.status || 'disconnected';
 const statusClass = status === 'connected' ? 'status-connected' : 
 status === 'connecting' ? 'status-connecting' :
 status === 'error' ? 'status-error' :
 status === 'disabled' ? 'status-disabled' : 'status-disconnected';
 const statusT = typeof window.t === 'function' ? window.t : (k) => k;
 const statusText = status === 'connected' ? statusT('mcp.connected') : 
 status === 'connecting' ? statusT('mcp.connecting') :
 status === 'error' ? statusT('mcp.connectionFailed') :
 status === 'disabled' ? statusT('mcp.disabled') : statusT('mcp.disconnected');
 const transport = server.config.transport || (server.config.command ? 'stdio' : 'http');
 const attrSafe = (v) => String(v == null ? '' : v).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
 const transportIcon = '<span class="external-mcp-transport" title="' + attrSafe(transport) + '">' + (typeof window.pyIcon === 'function' ? window.pyIcon(transport === 'stdio' ? 'terminal' : 'globe', { size: 14 }) : '') + '</span>';
 
 html += `
 <div class="external-mcp-item" data-mcp-name="${attrSafe(name)}">
 <div class="external-mcp-item-header">
 <div class="external-mcp-item-info">
 <h4>${transportIcon} ${escapeHtml(name)}${server.tool_count !== undefined && server.tool_count > 0 ? `<span class="tool-count-badge" title="${escapeHtml(statusT('mcp.toolCount'))}">${typeof window.pyIcon === 'function' ? window.pyIcon('wrench', { size: 12 }) : ''}${server.tool_count}</span>` : ''}</h4>
 <span class="external-mcp-status ${statusClass}">${statusText}</span>
 </div>
 <div class="external-mcp-item-actions">
 ${status === 'connected' || status === 'disconnected' || status === 'error' ? 
 `<button class="btn-small" id="btn-toggle-${attrSafe(name)}" onclick="toggleExternalMCP(this.closest('.external-mcp-item').dataset.mcpName, '${status}')" title="${status === 'connected' ? statusT('mcp.stopConnection') : statusT('mcp.startConnection')}">
 ${status === 'connected' ? (typeof window.pyIcon === 'function' ? window.pyIcon('pause', { size: 12 }) : '') + '<span>' + statusT('mcp.stop') + '</span>' : (typeof window.pyIcon === 'function' ? window.pyIcon('play', { size: 12 }) : '') + '<span>' + statusT('mcp.start') + '</span>'}
 </button>` : 
 status === 'connecting' ? 
 `<button class="btn-small" id="btn-toggle-${attrSafe(name)}" disabled>
 <span class="inline-spinner" aria-hidden="true"></span><span>${statusT('mcp.connecting')}</span>
 </button>` : ''}
 <button class="btn-small" onclick="editExternalMCP(this.closest('.external-mcp-item').dataset.mcpName)" title="${statusT('mcp.editConfig')}" ${status === 'connecting' ? 'disabled' : ''}>${typeof window.pyIcon === 'function' ? window.pyIcon('pencil', { size: 12 }) : ''}<span>${statusT('common.edit')}</span></button>
 <button class="btn-small btn-danger" onclick="deleteExternalMCP(this.closest('.external-mcp-item').dataset.mcpName)" title="${statusT('mcp.deleteConfig')}" ${status === 'connecting' ? 'disabled' : ''}>${typeof window.pyIcon === 'function' ? window.pyIcon('trash-2', { size: 12 }) : ''}<span>${statusT('common.delete')}</span></button>
 </div>
 </div>
 ${status === 'error' && server.error ? `
 <div class="external-mcp-error error-message">
 <strong>${statusT('mcp.connectionErrorLabel')}</strong> ${escapeHtml(server.error)}
 </div>` : ''}
 <div class="external-mcp-item-details">
 <div>
 <strong>${statusT('mcp.transportMode')}</strong>
 <span>${transportIcon} ${escapeHtml(transport.toUpperCase())}</span>
 </div>
 ${server.tool_count !== undefined && server.tool_count > 0 ? `
 <div>
 <strong>${statusT('mcp.toolCount')}</strong>
 <span style="font-weight: 600; color: var(--accent-color);">${statusT('mcp.toolsCountValue', { count: server.tool_count })}</span>
 </div>` : server.tool_count === 0 && status === 'connected' ? `
 <div>
 <strong>${statusT('mcp.toolCount')}</strong>
 <span style="color: var(--text-muted);">${statusT('mcp.noTools')}</span>
 </div>` : ''}
 ${server.config.description ? `
 <div>
 <strong>${statusT('mcp.description')}</strong>
 <span>${escapeHtml(server.config.description)}</span>
 </div>` : ''}
 ${server.config.timeout ? `
 <div>
 <strong>${statusT('mcp.timeout')}</strong>
 <span>${server.config.timeout} ${statusT('mcp.secondsUnit')}</span>
 </div>` : ''}
 ${transport === 'stdio' && server.config.command ? `
 <div>
 <strong>${statusT('mcp.command')}</strong>
 <span style="font-family: monospace; font-size: 0.8125rem;">${escapeHtml(server.config.command)}</span>
 </div>` : ''}
 ${transport === 'http' && server.config.url ? `
 <div>
 <strong>${statusT('mcp.urlLabel')}</strong>
 <span style="font-family: monospace; font-size: 0.8125rem; word-break: break-all;">${escapeHtml(server.config.url)}</span>
 </div>` : ''}
 </div>
 </div>
 `;
 }
 html += '</div>';
 list.innerHTML = html;
}
function renderExternalMCPStats(stats) {
 const statsEl = document.getElementById('external-mcp-stats');
 if (!statsEl) return;
 
 const total = stats.total || 0;
 const enabled = stats.enabled || 0;
 const disabled = stats.disabled || 0;
 const connected = stats.connected || 0;
 
 const statsT = typeof window.t === 'function' ? window.t : (k) => k;
 statsEl.innerHTML = `
 <span title="${statsT('mcp.totalCount')}">${statsT('mcp.totalCount')}: <strong>${total}</strong></span>
 <span title="${statsT('mcp.enabledCount')}">${statsT('mcp.enabledCount')}: <strong>${enabled}</strong></span>
 <span title="${statsT('mcp.disabledCount')}">${statsT('mcp.disabledCount')}: <strong>${disabled}</strong></span>
 <span title="${statsT('mcp.connectedCount')}">${statsT('mcp.connectedCount')}: <strong>${connected}</strong></span>
 `;
}
function showAddExternalMCPModal() {
 currentEditingMCPName = null;
 document.getElementById('external-mcp-modal-title').textContent = (typeof window.t === 'function' ? window.t('mcp.addExternalMCP') : 'Add external MCP');
 document.getElementById('external-mcp-json').value = '';
 document.getElementById('external-mcp-json-error').style.display = 'none';
 document.getElementById('external-mcp-json-error').textContent = '';
 document.getElementById('external-mcp-json').classList.remove('error');
 document.getElementById('external-mcp-modal').style.display = 'block';
}
function closeExternalMCPModal() {
 document.getElementById('external-mcp-modal').style.display = 'none';
 currentEditingMCPName = null;
}
async function editExternalMCP(name) {
 try {
 const response = await apiFetch(`/api/external-mcp/${encodeURIComponent(name)}`);
 if (!response.ok) {
 throw new Error(typeof window.t === 'function' ? window.t('mcp.getConfigFailed') : 'Failed to get config');
 }
 
 const server = await response.json();
 currentEditingMCPName = name;
 
 document.getElementById('external-mcp-modal-title').textContent = (typeof window.t === 'function' ? window.t('mcp.editExternalMCP') : 'Edit external MCP');
 const config = { ...server.config };
 delete config.tool_count;
 delete config.external_mcp_enable;
 const configObj = {};
 configObj[name] = config;
 const jsonStr = JSON.stringify(configObj, null, 2);
 document.getElementById('external-mcp-json').value = jsonStr;
 document.getElementById('external-mcp-json-error').style.display = 'none';
 document.getElementById('external-mcp-json-error').textContent = '';
 document.getElementById('external-mcp-json').classList.remove('error');
 
 document.getElementById('external-mcp-modal').style.display = 'block';
 } catch (error) {
 console.error('MCPfailed:', error);
 alert((typeof window.t === 'function' ? window.t('mcp.operationFailed') : 'Operation failed') + ': ' + error.message);
 }
}
function formatExternalMCPJSON() {
 const jsonTextarea = document.getElementById('external-mcp-json');
 const errorDiv = document.getElementById('external-mcp-json-error');
 
 try {
 const jsonStr = jsonTextarea.value.trim();
 if (!jsonStr) {
 errorDiv.textContent = (typeof window.t === 'function' ? window.t('mcp.jsonEmpty') : 'JSON cannot be empty');
 errorDiv.style.display = 'block';
 jsonTextarea.classList.add('error');
 return;
 }
 
 const parsed = JSON.parse(jsonStr);
 const formatted = JSON.stringify(parsed, null, 2);
 jsonTextarea.value = formatted;
 errorDiv.style.display = 'none';
 jsonTextarea.classList.remove('error');
 } catch (error) {
 errorDiv.textContent = (typeof window.t === 'function' ? window.t('mcp.jsonError') : 'JSON format error') + ': ' + error.message;
 errorDiv.style.display = 'block';
 jsonTextarea.classList.add('error');
 }
}
function loadExternalMCPExample() {
 const desc = (typeof window.t === 'function' ? window.t('externalMcpModal.exampleDescription') : 'Example description');
 const example = {
 "hexstrike-ai": {
 command: "python3",
 args: [
 "/path/to/script.py",
 "--server",
 "http://example.com"
 ],
 description: desc,
 timeout: 300
 },
 "cyberstrike-ai-http": {
 transport: "http",
 url: "http://127.0.0.1:8081/mcp"
 },
 "cyberstrike-ai-sse": {
 transport: "sse",
 url: "http://127.0.0.1:8081/mcp/sse"
 }
 };
 
 document.getElementById('external-mcp-json').value = JSON.stringify(example, null, 2);
 document.getElementById('external-mcp-json-error').style.display = 'none';
 document.getElementById('external-mcp-json').classList.remove('error');
}
async function saveExternalMCP() {
 const jsonTextarea = document.getElementById('external-mcp-json');
 const jsonStr = jsonTextarea.value.trim();
 const errorDiv = document.getElementById('external-mcp-json-error');
 
 if (!jsonStr) {
 errorDiv.textContent = (typeof window.t === 'function' ? window.t('mcp.jsonEmpty') : 'JSON cannot be empty');
 errorDiv.style.display = 'block';
 jsonTextarea.classList.add('error');
 jsonTextarea.focus();
 return;
 }
 
 let configObj;
 try {
 configObj = JSON.parse(jsonStr);
 } catch (error) {
 errorDiv.textContent = (typeof window.t === 'function' ? window.t('mcp.jsonError') : 'JSON format error') + ': ' + error.message;
 errorDiv.style.display = 'block';
 jsonTextarea.classList.add('error');
 jsonTextarea.focus();
 return;
 }
 
 const t = (typeof window.t === 'function' ? window.t : function (k, opts) { return k; });
 if (typeof configObj !== 'object' || Array.isArray(configObj) || configObj === null) {
 errorDiv.textContent = t('mcp.configMustBeObject');
 errorDiv.style.display = 'block';
 jsonTextarea.classList.add('error');
 return;
 }
 const names = Object.keys(configObj);
 if (names.length === 0) {
 errorDiv.textContent = t('mcp.configNeedOne');
 errorDiv.style.display = 'block';
 jsonTextarea.classList.add('error');
 return;
 }
 for (const name of names) {
 if (!name || name.trim() === '') {
 errorDiv.textContent = t('mcp.configNameEmpty');
 errorDiv.style.display = 'block';
 jsonTextarea.classList.add('error');
 return;
 }
 
 const config = configObj[name];
 if (typeof config !== 'object' || Array.isArray(config) || config === null) {
 errorDiv.textContent = t('mcp.configMustBeObj', { name: name });
 errorDiv.style.display = 'block';
 jsonTextarea.classList.add('error');
 return;
 }
 delete config.external_mcp_enable;
 const transport = config.transport || (config.command ? 'stdio' : config.url ? 'http' : '');
 if (!transport) {
 errorDiv.textContent = t('mcp.configNeedCommand', { name: name });
 errorDiv.style.display = 'block';
 jsonTextarea.classList.add('error');
 return;
 }
 
 if (transport === 'stdio' && !config.command) {
 errorDiv.textContent = t('mcp.configStdioNeedCommand', { name: name });
 errorDiv.style.display = 'block';
 jsonTextarea.classList.add('error');
 return;
 }
 
 if (transport === 'http' && !config.url) {
 errorDiv.textContent = t('mcp.configHttpNeedUrl', { name: name });
 errorDiv.style.display = 'block';
 jsonTextarea.classList.add('error');
 return;
 }
 
 if (transport === 'sse' && !config.url) {
 errorDiv.textContent = t('mcp.configSseNeedUrl', { name: name });
 errorDiv.style.display = 'block';
 jsonTextarea.classList.add('error');
 return;
 }
 }
 errorDiv.style.display = 'none';
 jsonTextarea.classList.remove('error');
 
 try {
 if (currentEditingMCPName) {
 if (!configObj[currentEditingMCPName]) {
 errorDiv.textContent = (typeof window.t === 'function' ? window.t('mcp.configEditMustContainName', { name: currentEditingMCPName }) : 'configerror: ,JSONconfigname "' + currentEditingMCPName + '"');
 errorDiv.style.display = 'block';
 jsonTextarea.classList.add('error');
 return;
 }
 
 const response = await apiFetch(`/api/external-mcp/${encodeURIComponent(currentEditingMCPName)}`, {
 method: 'PUT',
 headers: {
 'Content-Type': 'application/json',
 },
 body: JSON.stringify({ config: configObj[currentEditingMCPName] }),
 });
 
 if (!response.ok) {
 const error = await response.json();
 throw new Error(error.error || 'Save failed');
 }
 } else {
 for (const name of names) {
 const config = configObj[name];
 const response = await apiFetch(`/api/external-mcp/${encodeURIComponent(name)}`, {
 method: 'PUT',
 headers: {
 'Content-Type': 'application/json',
 },
 body: JSON.stringify({ config }),
 });
 
 if (!response.ok) {
 const error = await response.json();
 throw new Error(`save "${name}" failed: ${error.error || 'Unknown error'}`);
 }
 }
 }
 
 closeExternalMCPModal();
 await loadExternalMCPs();
 if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
 window.refreshMentionTools();
 }
 pollExternalMCPToolCount(null, 5);
 alert(typeof window.t === 'function' ? window.t('mcp.saveSuccess') : 'Saved');
 } catch (error) {
 console.error('saveMCPfailed:', error);
 errorDiv.textContent = (typeof window.t === 'function' ? window.t('mcp.operationFailed') : 'Operation failed') + ': ' + error.message;
 errorDiv.style.display = 'block';
 jsonTextarea.classList.add('error');
 }
}
async function deleteExternalMCP(name) {
 if (!confirm((typeof window.t === 'function' ? window.t('mcp.deleteExternalConfirm', { name: name }) : `deleteMCP "${name}" ？`))) {
 return;
 }
 
 try {
 const response = await apiFetch(`/api/external-mcp/${encodeURIComponent(name)}`, {
 method: 'DELETE',
 });
 
 if (!response.ok) {
 const error = await response.json();
 throw new Error(error.error || 'deletefailed');
 }
 
 await loadExternalMCPs();
 if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
 window.refreshMentionTools();
 }
 alert(typeof window.t === 'function' ? window.t('mcp.deleteSuccess') : 'Deleted');
 } catch (error) {
 console.error('deleteMCPfailed:', error);
 alert((typeof window.t === 'function' ? window.t('mcp.operationFailed') : 'Operation failed') + ': ' + error.message);
 }
}
async function toggleExternalMCP(name, currentStatus) {
 const action = currentStatus === 'connected' ? 'stop' : 'start';
 const buttonId = `btn-toggle-${name}`;
 const button = document.getElementById(buttonId);
 if (action === 'start' && button) {
 button.disabled = true;
 button.style.opacity = '0.6';
 button.style.cursor = 'not-allowed';
 button.innerHTML = '<span class="inline-spinner" aria-hidden="true"></span><span>Connecting...</span>';
 }
 
 try {
 const response = await apiFetch(`/api/external-mcp/${encodeURIComponent(name)}/${action}`, {
 method: 'POST',
 });
 
 if (!response.ok) {
 const error = await response.json();
 throw new Error(error.error || 'failed');
 }
 
 const result = await response.json();
 if (action === 'start') {
 try {
 const statusResponse = await apiFetch(`/api/external-mcp/${encodeURIComponent(name)}`);
 if (statusResponse.ok) {
 const statusData = await statusResponse.json();
 const status = statusData.status || 'disconnected';
 
 if (status === 'connected') {
 await loadExternalMCPs();
 if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
 window.refreshMentionTools();
 }
 pollExternalMCPToolCount(name, 10);
 return;
 }
 }
 } catch (error) {
 console.error('statusfailed:', error);
 }
 await pollExternalMCPStatus(name, 30); // 30(30 seconds)
 } else {
 await loadExternalMCPs();
 if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
 window.refreshMentionTools();
 }
 }
 } catch (error) {
 console.error('MCPstatusfailed:', error);
 alert((typeof window.t === 'function' ? window.t('mcp.operationFailed') : 'Operation failed') + ': ' + error.message);
 if (button) {
 button.disabled = false;
 button.style.opacity = '1';
 button.style.cursor = 'pointer';
 button.innerHTML = '▶ ';
 }
 await loadExternalMCPs();
 if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
 window.refreshMentionTools();
 }
 }
}
async function pollExternalMCPStatus(name, maxAttempts = 30) {
 let attempts = 0;
 const pollInterval = 1000; // 1 seconds
 
 while (attempts < maxAttempts) {
 await new Promise(resolve => setTimeout(resolve, pollInterval));
 
 try {
 const response = await apiFetch(`/api/external-mcp/${encodeURIComponent(name)}`);
 if (response.ok) {
 const data = await response.json();
 const status = data.status || 'disconnected';
 const buttonId = `btn-toggle-${name}`;
 const button = document.getElementById(buttonId);
 
 if (status === 'connected') {
 await loadExternalMCPs();
 if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
 window.refreshMentionTools();
 }
 pollExternalMCPToolCount(name, 10);
 return;
 } else if (status === 'error' || status === 'disconnected') {
 await loadExternalMCPs();
 if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
 window.refreshMentionTools();
 }
 if (status === 'error') {
 alert(typeof window.t === 'function' ? window.t('mcp.connectionFailedCheck') : 'Connection failed. Check config and network.');
 }
 return;
 } else if (status === 'connecting') {
 attempts++;
 continue;
 }
 }
 } catch (error) {
 console.error('statusfailed:', error);
 }
 
 attempts++;
 }
 await loadExternalMCPs();
 if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
 window.refreshMentionTools();
 }
 alert(typeof window.t === 'function' ? window.t('mcp.connectionTimeout') : 'Connection timeout. Check config and network.');
}
const originalOpenSettings = openSettings;
openSettings = async function() {
 await originalOpenSettings();
 await loadExternalMCPs();
};
document.addEventListener('languagechange', function () {
 try {
 const mcpPage = document.getElementById('page-mcp-management');
 if (mcpPage && mcpPage.classList.contains('active')) {
 if (typeof loadExternalMCPs === 'function') {
 loadExternalMCPs().catch(function () { /* ignore */ });
 }
 if (typeof updateToolsStats === 'function') {
 updateToolsStats().catch(function () { /* ignore */ });
 }
 }
 } catch (e) {
 console.warn('languagechange MCP refresh failed', e);
 }
});
