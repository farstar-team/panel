import { z } from 'zod';

export const StoreConfigSchema = z.object({
  id: z.number().int(),
  enabled: z.boolean(),
  paymentText: z.string().max(4000),
  supportURL: z.string(),
});
export const StorePlanSchema = z.object({
  id: z.number().int().nonnegative(),
  name: z.string().trim().min(1).max(150),
  price: z.number().int().min(1).max(1_000_000_000_000),
  quotaBytes: z
    .number()
    .int()
    .min(1)
    .max(100_000 * 2 ** 30),
  days: z.number().int().min(1).max(3650),
  enabled: z.boolean(),
});
export const StoreStateSchema = z.object({
  config: StoreConfigSchema,
  plans: z.array(StorePlanSchema),
});
export const StorePlanFormSchema = StorePlanSchema.omit({ quotaBytes: true }).extend({
  quotaGB: z.number().positive().max(100_000),
});
export type StoreConfig = z.infer<typeof StoreConfigSchema>;
export type StorePlan = z.infer<typeof StorePlanSchema>;
export type StorePlanForm = z.infer<typeof StorePlanFormSchema>;
