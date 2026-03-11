<script lang="ts" setup>
import { ref, onMounted, onUnmounted, nextTick, computed } from 'vue';
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime';
import { InitializeFile } from '../wailsjs/go/main/App';
import MarkdownIt from 'markdown-it';
import hljs from 'highlight.js';
import mermaid from 'mermaid';
import AnsiToHtml from 'ansi-to-html';
import TreeNode from './components/TreeNode.vue';
import { GetDirectoryTree, GetParentDir } from '../wailsjs/go/main/App';

// Styles for GitHub markdown and syntax highlighting
import 'github-markdown-css/github-markdown.css';
import 'highlight.js/styles/github.css';

const markdownHtml = ref<string>('');
const isError = ref<boolean>(false);
const treeNodes = ref<any[]>([]);
const currentDir = ref<string>('');
const isWordWrap = ref<boolean>(false);

const currentDirName = computed(() => {
  if (!currentDir.value) return 'Drives';
  const parts = currentDir.value.split(/[\\/]/).filter(Boolean);
  if (parts.length === 0) return currentDir.value;
  if (parts.length === 1 && parts[0].endsWith(':')) return parts[0] + '\\';
  return parts[parts.length - 1];
});

const navigateUp = async () => {
  try {
    const parentDir = await GetParentDir(currentDir.value);
    if (parentDir && parentDir !== currentDir.value) {
      currentDir.value = parentDir;
      treeNodes.value = await GetDirectoryTree(parentDir);
    }
  } catch (e) {
    console.error("Failed to navigate up", e);
  }
};

const navigateToDir = async (path: string) => {
  try {
    currentDir.value = path;
    treeNodes.value = await GetDirectoryTree(path);
  } catch (e) {
    console.error("Failed to navigate to dir", e);
  }
};

const sidebarWidth = ref<number>(300);
const isResizing = ref<boolean>(false);

const startResize = (e: MouseEvent) => {
  isResizing.value = true;
  document.addEventListener('mousemove', doResize);
  document.addEventListener('mouseup', stopResize);
  document.body.style.cursor = 'col-resize';
  document.body.style.userSelect = 'none';
};

const doResize = (e: MouseEvent) => {
  if (isResizing.value) {
    let newWidth = e.clientX;
    // adding constrains
    if (newWidth < 150) newWidth = 150;
    if (newWidth > 800) newWidth = 800;
    sidebarWidth.value = newWidth;
  }
};

const stopResize = () => {
  isResizing.value = false;
  document.removeEventListener('mousemove', doResize);
  document.removeEventListener('mouseup', stopResize);
  document.body.style.cursor = '';
  document.body.style.userSelect = '';
};

mermaid.initialize({ startOnLoad: false, theme: 'default' });

// Initialize markdown-it with highlight.js integration
const md = new MarkdownIt({
  html: true,
  linkify: true,
  typographer: true,
  highlight: function (str: string, lang: string) {
    if (lang === 'mermaid') {
      return `<div class="mermaid">${str}</div>`;
    }
    if (lang && hljs.getLanguage(lang)) {
      try {
        return hljs.highlight(str, { language: lang }).value;
      } catch (__) {}
    }
    return ''; // use external default escaping
  }
});

// Configure MarkdownIt to open links in a new window
const defaultRender = md.renderer.rules.link_open || function(tokens, idx, options, env, self) {
  return self.renderToken(tokens, idx, options);
};
md.renderer.rules.link_open = function (tokens, idx, options, env, self) {
  const aIndex = tokens[idx].attrIndex('target');
  if (aIndex < 0) {
    tokens[idx].attrPush(['target', '_blank']);
  } else {
    if (tokens[idx].attrs && tokens[idx].attrs![aIndex]) {
      tokens[idx].attrs![aIndex][1] = '_blank';
    }
  }
  return defaultRender(tokens, idx, options, env, self);
};

const ansiConvert = new AnsiToHtml({
  escapeXML: false,
  newline: false,
  fg: '#24292f',
  bg: '#ffffff',
  colors: {
    1: '#cb2431',  // [31m (Red text)
    6: '#1b7c83',  // [36m / [46m (Cyan / Cyan background)
    10: '#28a745', // [92m (Bright Green text)
    14: '#005cc5', // [96m (Bright Cyan text) -> make it a darker solid blue
    15: '#f6f8fa'  // [97m / [107m (Bright White text / Bright White background)
  }
});

onMounted(() => {
  // Listen for markdown updates from the Go backend
  EventsOn('markdown-updated', async (markdownContent: string) => {
    isError.value = markdownContent.startsWith('# Error\n');
    const renderedHtml = md.render(markdownContent);
    // Convert ANSI escape codes to HTML tags
    markdownHtml.value = ansiConvert.toHtml(renderedHtml);
    await nextTick();
    try {
      mermaid.run({ querySelector: '.mermaid', suppressErrors: true });
    } catch (e) {
      console.error('Mermaid render error', e);
    }
  });

  EventsOn('set-initial-dir', async (dirPath: string) => {
    currentDir.value = dirPath;
    try {
      const nodes = await GetDirectoryTree(dirPath);
      treeNodes.value = nodes;
    } catch (e) {
      console.error("Failed to load initial directory", e);
    }
  });

  // Tell the backend we are ready to receive the initial file
  InitializeFile();
});

onUnmounted(() => {
  EventsOff('markdown-updated');
  EventsOff('set-initial-dir');
});
</script>

<template>
  <div class="layout-container" :class="{ 'is-resizing': isResizing }">
    <div class="sidebar" :style="{ width: sidebarWidth + 'px' }">
      <div class="sidebar-header">
        <button class="up-btn" @click="navigateUp" title="Up to parent directory">⬆️</button>
        <span class="sidebar-title" :title="currentDir">{{ currentDirName }}</span>
      </div>
      <div class="sidebar-content">
        <TreeNode 
          v-for="node in treeNodes" 
          :key="node.path" 
          :node="node" 
          @navigate="navigateToDir"
        />
      </div>
    </div>
    
    <div class="resizer" @mousedown="startResize" :class="{ active: isResizing }"></div>
    
    <div class="main-content wrapper" :class="{ 'error-wrapper': isError }">
      <div class="main-toolbar" v-if="markdownHtml">
        <label class="wrap-checkbox">
          <input type="checkbox" v-model="isWordWrap"> 右端で折り返す
        </label>
      </div>
      <div v-if="!markdownHtml" class="loading-state">
        <p>Waiting for markdown content...</p>
      </div>
      <div 
        v-else
        class="markdown-body custom-markdown" 
        :class="{ 'word-wrap': isWordWrap }"
        v-html="markdownHtml"
      ></div>
    </div>
  </div>
</template>

<style>
/* Base window and scrollbar styling */
html, body {
  margin: 0;
  padding: 0;
  width: 100%;
  height: 100%;
  background-color: #ffffff; /* GitHub light mode background */
  color: #24292f;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  overflow-y: auto;
}

/* Custom scrollbar to match dark theme */
::-webkit-scrollbar {
  width: 10px;
  height: 10px;
}
::-webkit-scrollbar-track {
  background: #ffffff; 
}
::-webkit-scrollbar-thumb {
  background: #c1c1c1; 
  border-radius: 5px;
}
::-webkit-scrollbar-thumb:hover {
  background: #a8a8a8; 
}

.layout-container {
  display: flex;
  height: 100vh;
  width: 100%;
}

.layout-container.is-resizing {
  user-select: none;
}

.sidebar {
  min-width: 150px;
  max-width: 800px;
  background-color: #f6f8fa;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.resizer {
  width: 4px;
  cursor: col-resize;
  background-color: transparent;
  border-right: 1px solid #d0d7de;
  transition: background-color 0.2s;
  z-index: 10;
}

.resizer:hover, .resizer.active {
  background-color: #0969da;
  border-right: none;
  width: 5px;
}

.sidebar-header {
  padding: 10px 16px;
  border-bottom: 1px solid #d0d7de;
  font-weight: 600;
  font-size: 14px;
  color: #24292f;
  display: flex;
  align-items: center;
  gap: 8px;
}

.up-btn {
  background: none;
  border: none;
  cursor: pointer;
  font-size: 14px;
  padding: 4px 6px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.up-btn:hover {
  background-color: #d0d7de;
}

.sidebar-content {
  flex: 1;
  overflow-y: auto;
  padding: 10px;
}

.main-content {
  flex: 1;
  overflow-y: auto;
}

.wrapper {
  padding: 32px;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  justify-content: flex-start;
}

.main-toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 24px;
}

.wrap-checkbox {
  font-size: 13px;
  color: #57606a;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
  user-select: none;
}

.error-wrapper {
  background-color: #440000;
}

.loading-state {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 80vh;
  color: #8b949e;
  font-size: 1.2rem;
}

/* Constrain the maximum width to make reading easier */
.custom-markdown {
  max-width: 100%;
  width: 100%;
  margin: 0;
  text-align: left;
  background-color: transparent !important;
}

.custom-markdown.word-wrap {
  word-wrap: break-word !important;
  overflow-wrap: break-word !important;
}

.custom-markdown.word-wrap pre,
.custom-markdown.word-wrap code {
  white-space: pre-wrap !important;
  word-wrap: break-word !important;
}
</style>
