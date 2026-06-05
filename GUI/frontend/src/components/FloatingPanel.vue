<script setup>
import { computed } from 'vue'
import { CancelRecording, HideWindow, TogglePause, ToggleRecording } from '../api'

const props = defineProps({ status: { type: Object, required: true } })
defineEmits(['open-settings'])

const isRecording = computed(() => props.status.state === 'Recording' || props.status.state === 'Paused')
const isPaused = computed(() => props.status.state === 'Paused')
const isBusy = computed(() => props.status.state === 'Uploading')

async function toggleRecording() {
  await ToggleRecording()
}

async function togglePause() {
  if (isRecording.value) await TogglePause()
}

async function cancel() {
  if (isRecording.value) await CancelRecording()
}
</script>

<template>
  <main class="panel">
    <header class="drag-zone">
      <span class="dot" :class="status.state.toLowerCase()"></span>
      <span>{{ status.state }}</span>
      <button class="ghost" title="Hide" @click="HideWindow">—</button>
    </header>

    <section class="controls">
      <button class="record-button" :class="{ active: isRecording, uploading: isBusy }" :disabled="isBusy" @click="toggleRecording">
        <span class="mic">●</span>
        <span>{{ isRecording ? 'Stop' : isBusy ? 'Uploading' : 'Record' }}</span>
      </button>
      <button class="pause-button" :class="{ active: isPaused }" :disabled="!isRecording" @click="togglePause">
        <span>{{ isPaused ? '▶' : 'Ⅱ' }}</span>
        <span>{{ isPaused ? 'Resume' : 'Pause' }}</span>
      </button>
    </section>

    <footer>
      <span class="message">{{ status.message }}</span>
      <nav>
        <button class="link" @click="$emit('open-settings')">Settings</button>
        <button class="link danger" :disabled="!isRecording" @click="cancel">Cancel</button>
      </nav>
    </footer>
  </main>
</template>
