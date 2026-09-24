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
The Render an Edit was made from.
_Avoid_: Parent, original

**Edit region**:
An area the user marks on a Render for an Edit, paired with a free-text instruction.
_Avoid_: Mask (a Mask belongs to the pre-render screenshot and carries an asset)

**Mask**:
An area painted on a view's screenshot before rendering, tied to an asset.

## Relationships

- An **Edit** is a **Render** and has exactly one **Source render**
- A **Render** can have many **Edits**, and an **Edit** can itself be edited
- An **Edit** is made of one or more **Edit regions**, each with its own instruction
- Render history shows an **Edit** directly under its **Source render**, indented and joined by an arrow

## Example dialogue

> **User:** "I like this render but the lamp is wrong."
> **Dev:** "Mark the lamp as an **Edit region** and describe it. That creates an **Edit**, a new **Render**, and the **Source render** stays as it was."
