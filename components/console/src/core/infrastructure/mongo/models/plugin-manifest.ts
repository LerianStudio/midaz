import { PluginManifestEntity } from '@/core/domain/entities/plugin-manifest-entity'
import mongoose, { Document, Model, Schema } from 'mongoose'
import { v4 as uuidv4 } from 'uuid'

export interface PluginManifestDocument
  extends Omit<PluginManifestEntity, 'id'>,
    Document {
  createdAt: Date
  updatedAt: Date
}

const pluginManifestSchema = new Schema<PluginManifestDocument>(
  {
    id: {
      type: String,
      required: true,
      index: true,
      unique: true,
      default: () => uuidv4()
    },
    name: { type: String, required: true, index: true, unique: true },
    title: { type: String, required: true },
    description: { type: String, required: true },
    version: { type: String, required: true },
    route: { type: String, required: true },
    entry: { type: String, required: true },
    healthcheck: { type: String, required: true },
    host: { type: String, required: true },
    icon: { type: String, required: true },
    enabled: { type: Boolean, required: true },
    author: { type: String, required: true }
  },
  {
    timestamps: true,
    versionKey: false
  }
)

const MODEL_NAME = 'Plugin-Manifest'
const PluginManifestModel: Model<PluginManifestDocument> =
  mongoose.models[MODEL_NAME] ||
  mongoose.model(MODEL_NAME, pluginManifestSchema)

export default PluginManifestModel
