/**
 * @file Organization Avatar MongoDB Model
 * @description Defines the MongoDB schema and model for organization avatars.
 * This model is used to store and retrieve organization avatar images in the database.
 */

import mongoose, { Schema, Document, Model } from 'mongoose'
import { OrganizationAvatarEntity } from '@/core/domain/entities/organization-avatar-entity'

/**
 * OrganizationAvatarDocument interface
 * @interface
 * @extends OrganizationAvatarEntity - Core domain entity for organization avatars
 * @extends Document - Mongoose document interface
 * @description Represents a MongoDB document for organization avatars with additional
 * Mongoose-specific fields like timestamps.
 */
export interface OrganizationAvatarDocument
  extends OrganizationAvatarEntity,
    Document {
  createdAt: Date
  updatedAt: Date
}

/**
 * MongoDB schema for organization avatars
 * @description Defines the structure and validation rules for organization avatar documents
 */
const organizationAvatarSchema = new Schema<OrganizationAvatarDocument>(
  {
    organizationId: { type: String, required: true, index: true, unique: true },
    imageBase64: { type: String, required: true }
  },
  {
    timestamps: true,
    versionKey: false
  }
)

/**
 * Mongoose model for organization avatars
 * @description Compiled model used to perform CRUD operations on organization avatar documents
 */
const OrganizationAvatarModel: Model<OrganizationAvatarDocument> =
  mongoose.model('OrganizationAvatar', organizationAvatarSchema)

export default OrganizationAvatarModel
