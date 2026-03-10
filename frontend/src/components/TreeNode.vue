<script lang="ts" setup>
import { ref, watch } from 'vue';
import { GetDirectoryTree, LoadFile } from '../../wailsjs/go/main/App';

interface FileNode {
  name: string;
  path: string;
  isDir: boolean;
  children?: FileNode[];
}

const props = defineProps<{
  node: FileNode;
}>();

const isOpen = ref(false);

const toggle = async () => {
  if (props.node.isDir) {
    isOpen.value = !isOpen.value;
    if (isOpen.value && (!props.node.children || props.node.children.length === 0)) {
        try {
            const children = await GetDirectoryTree(props.node.path);
            props.node.children = children;
        } catch (e) {
            console.error("Failed to load children", e);
        }
    }
  } else if (props.node.name.toLowerCase().endsWith('.md')) {
    LoadFile(props.node.path);
  }
};
</script>

<template>
  <div class="tree-node">
    <div 
        class="node-label" 
        @click="toggle"
        :class="{ 'is-dir': node.isDir, 'is-md': !node.isDir && node.name.toLowerCase().endsWith('.md') }"
    >
      <span class="icon">{{ node.isDir ? (isOpen ? '📂' : '📁') : '📄' }}</span>
      {{ node.name }}
    </div>
    <div v-if="node.isDir && isOpen && node.children" class="node-children">
      <TreeNode 
          v-for="child in node.children" 
          :key="child.path" 
          :node="child" 
      />
    </div>
  </div>
</template>

<style scoped>
.tree-node {
  text-align: left;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  font-size: 14px;
}
.node-label {
  cursor: pointer;
  padding: 4px 8px;
  display: flex;
  align-items: center;
  user-select: none;
  border-radius: 4px;
  color: #000000;
}
.node-label:hover {
  background-color: #f0f0f0;
}
.node-label.is-md {
  font-weight: 500;
  color: #0366d6;
}
.icon {
  margin-right: 6px;
  font-size: 16px;
}
.node-children {
  padding-left: 20px;
}
</style>
