# Render AI

Turns a screenshot of a 3D scene into photorealistic renders, guided by assets painted onto the screenshot, and lets the user refine a finished render.

## Language

**Render**:
One generated image in a view's history, never overwritten once created.
_Avoid_: Result, output, generation

**Edit**:
A Render derived from another Render by changing only marked areas of it.
_Avoid_: Update, revision, inpaint

**Source render**:
The Render an Edit or an Upscale was made from.
_Avoid_: Parent, original

**Upscale**:
A Render that is the 4K version of another Render: the same picture, colors and lighting, with finer detail. It exists because generating the same request again at 4K would give a different picture.
_Avoid_: Enhance, 4K render (a 4K Render made from a screenshot is not an Upscale)

**Edit region**:
An area the user marks on a Render for an Edit, paired with a free-text instruction.
_Avoid_: Mask (a Mask belongs to the pre-render screenshot and carries an asset)

**Attempt**:
One model call made to produce a Render. A Render that does not follow its screenshot is discarded and made again, so a Render can be the result of several Attempts; only the kept one is ever shown.
_Avoid_: Retry, regen (in user-facing text)

**Mask**:
An area painted on a view's screenshot before rendering, tied to an asset.

**Notification**:
One entry in the header bell for a job that makes a Render, an Edit or an Upscale: shown while the job runs, and kept for a day after it finishes. It is unread until the user opens it, and opening it goes to the project, view and Render it made.
_Avoid_: Alert, toast (a toast is only how a finished Notification may first show up), message

## Relationships

- An **Edit** is a **Render** and has exactly one **Source render**
- An **Upscale** is a **Render** and has exactly one **Source render**; it is never an **Edit**
- A **Render** can have many **Edits** and many **Upscales**, and an **Edit** can itself be edited or upscaled
- A 4K **Render** cannot be upscaled
- An **Edit** is made of one or more **Edit regions**, each with its own instruction
- A **Notification** belongs to exactly one job, and so to one view of one project; a job that makes several Renders (variations) is still one **Notification**
- Render history shows an **Edit** or an **Upscale** directly under its **Source render**, indented and joined by an arrow

## Example dialogue

> **User:** "I like this render but the lamp is wrong."
> **Dev:** "Mark the lamp as an **Edit region** and describe it. That creates an **Edit**, a new **Render**, and the **Source render** stays as it was."
