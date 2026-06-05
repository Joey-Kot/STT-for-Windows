<script setup>
import { computed, onMounted, ref } from 'vue'
import { ConfigPath, LoadConfigJSON, SaveConfigJSON } from '../api'

const props = defineProps({ status: { type: Object, required: true } })
const emit = defineEmits(['close'])
const raw = ref('')
const path = ref('')
const error = ref('')
const saving = ref(false)
const tab = ref('JSON')
const tabs = ['API', 'Audio', 'Network', 'Hotkeys', 'Cache', 'Notifications', 'Debug', 'Advanced', 'JSON']
const canSave = computed(() => ['Idle', 'Error'].includes(props.status.state))

onMounted(async () => {
  path.value = await ConfigPath()
  raw.value = await LoadConfigJSON()
})

async function save() {
  error.value = ''
  saving.value = true
  try {
    await SaveConfigJSON(raw.value)
    emit('close')
  } catch (err) {
    error.value = String(err)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="settings-backdrop">
    <section class="settings">
      <aside>
        <strong>Settings</strong>
        <button v-for="name in tabs" :key="name" :class="{ selected: tab === name }" @click="tab = name">{{ name }}</button>
      </aside>
      <article>
        <header>
          <div>
            <h2>{{ tab }} Config</h2>
            <small>{{ path }}</small>
          </div>
          <button class="ghost" @click="emit('close')">×</button>
        </header>
        <p v-if="tab !== 'JSON'" class="hint">MVP uses a safe JSON editor while preserving the original CLI-compatible schema. Token values stay in the JSON file and are not logged.</p>
        <textarea v-model="raw" spellcheck="false"></textarea>
        <p v-if="!canSave" class="warning">Wait until recording/uploading finishes before saving.</p>
        <p v-if="error" class="error">{{ error }}</p>
        <footer>
          <button @click="emit('close')">Cancel</button>
          <button class="primary" :disabled="saving || !canSave" @click="save">{{ saving ? 'Saving…' : 'Save' }}</button>
        </footer>
      </article>
    </section>
  </div>
</template>
