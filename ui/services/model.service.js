// Model Service for WiFi-DensePose UI
// Manages model loading, listing, LoRA profiles, and lifecycle events.

import { apiService } from './api.service.js';

export class ModelService {
  constructor() {
    this.activeModel = null;
    this.listeners = {};
    this.logger = this.createLogger();
  }

  createLogger() {
    const fmt = (args) => args.map((a) => (
      a && typeof a === 'object'
        ? JSON.stringify(a)
        : a
    ));
    return {
      debug: (...args) => console.debug('[MODEL-DEBUG]', new Date().toISOString(), ...fmt(args)),
      info: (...args) => console.info('[MODEL-INFO]', new Date().toISOString(), ...fmt(args)),
      warn: (...args) => console.warn('[MODEL-WARN]', new Date().toISOString(), ...fmt(args)),
      error: (...args) => console.error('[MODEL-ERROR]', new Date().toISOString(), ...fmt(args))
    };
  }

  // --- Event emitter helpers ---

  on(event, callback) {
    if (!this.listeners[event]) {
      this.listeners[event] = [];
    }
    this.listeners[event].push(callback);
    return () => this.off(event, callback);
  }

  off(event, callback) {
    if (!this.listeners[event]) return;
    this.listeners[event] = this.listeners[event].filter(cb => cb !== callback);
  }

  emit(event, data) {
    if (!this.listeners[event]) return;
    this.listeners[event].forEach(cb => {
      try { cb(data); } catch (err) { this.logger.error('Listener error', { event, err }); }
    });
  }

  // --- API methods ---

  async listModels() {
    try {
      const data = await apiService.get('/api/v1/models');
      this.logger.info('Listed models', { count: data?.models?.length ?? 0 });
      return data;
    } catch (error) {
      this.logger.error('Failed to list models', { error: error.message });
      throw error;
    }
  }

  async getModel(id) {
    // wifi-densepose:latest returns HTTP 404 for GET /api/v1/models/:id
    // (empty body; route not registered). Always resolve via list instead.
    const listed = await this.listModels();
    const found = (listed?.models || []).find(
      (m) => (m.id || m.model_id || m.name) === id
    );
    if (found) return found;
    const err = new Error(`Model not in list: ${id}`);
    this.logger.error('Failed to get model', { id, error: err.message });
    throw err;
  }

  async loadModel(modelId) {
    try {
      this.logger.info('Loading model', { modelId });
      const data = await apiService.post('/api/v1/models/load', { model_id: modelId });
      this.activeModel = { model_id: modelId };
      this.emit('model-loaded', { model_id: modelId });
      return data;
    } catch (error) {
      this.logger.error('Failed to load model', { modelId, error: error.message });
      throw error;
    }
  }

  async unloadModel() {
    try {
      this.logger.info('Unloading model');
      const data = await apiService.post('/api/v1/models/unload', {});
      this.activeModel = null;
      this.emit('model-unloaded', {});
      return data;
    } catch (error) {
      this.logger.error('Failed to unload model', { error: error.message });
      throw error;
    }
  }

  async getActiveModel() {
    try {
      const data = await apiService.get('/api/v1/models/active');
      // Normalize { active: { id } } (current server) vs flat { model_id }
      let active = data;
      if (data && data.active) active = data.active;
      if (active && !active.model_id && (active.id || active.name)) {
        active = { ...active, model_id: active.id || active.name };
      }
      this.activeModel = active || null;
      return this.activeModel;
    } catch (error) {
      if (error.status === 404) {
        this.activeModel = null;
        return null;
      }
      this.logger.error('Failed to get active model', { error: error.message });
      throw error;
    }
  }

  async activateLoraProfile(modelId, profileName) {
    try {
      this.logger.info('Activating LoRA profile', { modelId, profileName });
      const data = await apiService.post(
        '/api/v1/models/lora/activate',
        { model_id: modelId, profile_name: profileName }
      );
      this.emit('lora-activated', { model_id: modelId, profile: profileName });
      return data;
    } catch (error) {
      this.logger.error('Failed to activate LoRA', { modelId, profileName, error: error.message });
      throw error;
    }
  }

  async getLoraProfiles() {
    try {
      const data = await apiService.get('/api/v1/models/lora/profiles');
      return data?.profiles ?? [];
    } catch (error) {
      this.logger.error('Failed to get LoRA profiles', { error: error.message });
      throw error;
    }
  }

  async deleteModel(id) {
    try {
      this.logger.info('Deleting model', { id });
      const data = await apiService.delete(`/api/v1/models/${encodeURIComponent(id)}`);
      return data;
    } catch (error) {
      this.logger.error('Failed to delete model', { id, error: error.message });
      throw error;
    }
  }

  dispose() {
    this.listeners = {};
    this.activeModel = null;
    this.logger.info('ModelService disposed');
  }
}

// Create singleton instance
export const modelService = new ModelService();
