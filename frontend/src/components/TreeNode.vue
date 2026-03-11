<script lang="ts" setup>
import { LoadFile } from '../../wailsjs/go/main/App';

interface FileNode {
  name: string;
  path: string;
  isDir: boolean;
}

const props = defineProps<{
  node: FileNode;
}>();

const emit = defineEmits<{
  (e: 'navigate', path: string): void
}>();

const handleInteraction = () => {
  if (props.node.isDir) {
    emit('navigate', props.node.path);
  } else {
    LoadFile(props.node.path);
  }
};
</script>

<template>
  <div class="tree-node">
    <div 
        class="node-label" 
        @click="handleInteraction"
        :class="{ 'is-dir': node.isDir, 'is-file': !node.isDir }"
    >
      <span class="icon">{{ node.isDir ? '📁' : '📄' }}</span>
      {{ node.name }}
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
.node-label.is-file {
  font-weight: 400;
  color: #000000;
}
.icon {
  margin-right: 6px;
  font-size: 16px;
}
</style>
