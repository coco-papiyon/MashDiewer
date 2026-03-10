<script lang="ts" setup>
import { ref, onMounted, onUnmounted, nextTick } from 'vue';
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime';
import { InitializeFile } from '../wailsjs/go/main/App';
import MarkdownIt from 'markdown-it';
import hljs from 'highlight.js';
import mermaid from 'mermaid';
import TreeNode from './components/TreeNode.vue';
import { GetDirectoryTree } from '../wailsjs/go/main/App';

// Styles for GitHub markdown and syntax highlighting
import 'github-markdown-css/github-markdown.css';
import 'highlight.js/styles/github.css';

const markdownHtml = ref<string>('');
const isError = ref<boolean>(false);
const treeNodes = ref<any[]>([]);
const currentDir = ref<string>('');

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

onMounted(() => {
  // Listen for markdown updates from the Go backend
  EventsOn('markdown-updated', async (markdownContent: string) => {
    isError.value = markdownContent.startsWith('# Error\n');
    markdownHtml.value = md.render(markdownContent);
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
  <div class="layout-container">
    <div class="sidebar">
      <div class="sidebar-header">
        <span class="sidebar-title">Explorer</span>
      </div>
      <div class="sidebar-content">
        <TreeNode 
          v-for="node in treeNodes" 
          :key="node.path" 
          :node="node" 
        />
      </div>
    </div>
    
    <div class="main-content wrapper" :class="{ 'error-wrapper': isError }">
    <div v-if="!markdownHtml" class="loading-state">
      <p>Waiting for markdown content...</p>
    </div>
    <div 
      v-else
      class="markdown-body custom-markdown" 
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

.sidebar {
  width: 300px;
  min-width: 200px;
  background-color: #f6f8fa;
  border-right: 1px solid #d0d7de;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.sidebar-header {
  padding: 10px 16px;
  border-bottom: 1px solid #d0d7de;
  font-weight: 600;
  font-size: 14px;
  color: #24292f;
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
  justify-content: flex-start;
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
</style>
