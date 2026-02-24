# 1. Research
## 1.1 Prompt 01 Do Research on Codebase
Source: ```text https://boristane.com/blog/how-i-use-claude-code/```

```text
Read this folder in depth. Understand how it works deeply, what it does and all its specificities, when that’s done. Write a detailed report of your learnings and findings in research.md
```

## 1.2 Prompt 02 Do Research on Codebase
Source: ```text https://boristane.com/blog/how-i-use-claude-code/```
```text
Study the notification system in great details. Understand the intricacies of it and write a detailed research.md document with everything there is to know about how notifications work
```

## 1.3 Prompt 03 - Find Bugs
Source: ```text https://boristane.com/blog/how-i-use-claude-code/```
```text
Go through the task scheduling flow, understand it deeply and look for potential bugs. There definitely are bugs in the system as it sometimes runs tasks that should have been cancelled. Keep researching the flow until you find all the bugs. Don’t stop until all the bugs are found. When you’re done, write a detailed report of your findings in research.md
```

# 2. Planning

## 2.1 Prompt - Write a Plan with Requirement Doc
Source: ```text https://boristane.com/blog/how-i-use-claude-code/```

You asked a Coding Agent to write a requirement doc. This is the next step.

```text
I want to build a new feature <name and description> that extends the system to perform <business outcome>. Refer to <requirement-document> for more information about the feature. Write a detailed plan.md document outlining how to implement this, include code snippets and save it to <file-path>.
```

## 2.2 Prompt - Write a Plan without Requirement Doc
Make sure you describe your feature in great details!

```text
The list endpoint should support cursor-based pagination instead of offset. Write a detailed plan.md for how to achieve this. Read source files before suggesting changes. Base the plan on the actual codebase
```

## 2.3. Use Concrete Examples
Most LLMs work dramatically better when it has a concrete reference implementation. When you ask a Coding Assistant to write a plan, it is a good idea to share a concrete example, if you have one, alongside the plan-in-writing. For instance, if want to add sortable IDs, and you find a good implementation in your codebase or from an open-source project but your requirements do not 100% match the referenced one, paste the ID generation code from that project that does it well and say “this is how they do sortable IDs. Write a plan.md explaining how we can adopt a similar approach.”

## 2.4. Add a TODO List 
Before implementing a feature, request a granular task breakdown:
```text
Add a detailed todo list to the plan, with all the phases and individual tasks necessary to complete the plan - don’t implement yet
```

# 3. Review and Revise
ALWAYS review the docs Coding Assistant wrote. Add your comment and correction. Go back to the Coding Assistant to review it, and repeat until you are satisfied with the doc (source: ```text https://boristane.com/blog/how-i-use-claude-code/```).

<div style="text-align: center;">
    <img src="../../../Resources/images/image_2026022201.png" alt="Description" width="400">
</div>

**Examples**
- “use drizzle:generate for migrations, not raw SQL” — domain knowledge Claude doesn’t have
- “no — this should be a PATCH, not a PUT” — correcting a wrong assumption
- “remove this section entirely, we don’t need caching here” — rejecting a proposed approach
- “the queue consumer already handles retries, so this retry logic is redundant. remove it and just let it fail” — explaining why something should change
- “this is wrong, the visibility field needs to be on the list itself, not on individual items. when a list is public, all items are public. restructure the schema section accordingly” — redirecting an entire section of the plan

# 4. Implementation

When the plan is ready, issue the implementation command. Use the following prompt across sessions:

## Prompt 
```text
Implement all the features based on the requirement doc (<file-path>) and the planning doc (<file-path>). When you’re done with a task or phase, mark it as completed in the plan document. Do not stop until all tasks and phases are completed. Do not use any or unknown types. If new types are required, ask before proceed. Continuously run type check to make sure you’re not introducing new issues.
```

This single prompt encodes everything that matters:

- “implement all the features”: do everything in the plan, don’t cherry-pick
- “mark it as completed in the plan document”: the plan is the source of truth for progress
- “do not stop until all tasks and phases are completed”: don’t pause for confirmation mid-flow
- “do not use any or unknown types”: maintain strict typing
- “continuously run type check”: catch problems early, not at the end

# 5. Feedback and Iteration
Once Coding Assistant finishes executing the plan, your role shifts from architect to supervisor.

<div style="text-align: center;">
    <img src="../../../Resources/images/image_2026022202.png" alt="Description" width="600">
</div>

Prompt the Coding Assistant to correct errors/misimplementations or adding missing tasks.

# 6. Skills
Coding should be done by skills, or preferrably agents.
- Requirement Skill/Agent
- Requirement Review Skill/Agent
- Planning Skill/Agent
- Planning Review Skill/Agent
- Implementation Skill/Agent
- Implementation Review Skill/Agent
- Test Skill/Agent
- Test Review Skill/Agent

## 6.1 Master Your Coding Skills/Agents 
Do not just use skills/agents from others. But do not write skills/agents from scratch, either. Find a good one, install it on your system, study it, fully understand it and be ready to modify it to fit your special needs.

- In the above development cycle, at any stage, if you find your Coding Assistant did not use the right skill(s) or agent(s), figure out why it did not use the skill(s) or agent(s). Either improve your prompts, or improve your skills/agents.
- If the Coding Assistant used skills/agents but did not achieve the idea goals, study the reasons, trying to fix the problems in the involved skills/agents.

# 7. Staying in the Driver Seat 
Even though you delegate execution to your Coding Assistant, you never give it total autonomy over what gets built. You do the vast majority of the active steering in the development cycle.

This matters because Coding Assistants will sometimes propose solutions that are technically correct but wrong for the project. Maybe the approach is over-engineered, or it changes a public API signature that other parts of the system depend on, or it picks a more complex option when a simpler one would do. You have context about the broader system, the product direction, and the engineering culture that the Coding Assistant doesn’t.

<div style="text-align: center;">
    <img src="../../../Resources/images/image_2026022203.png" alt="Description" width="600">
</div>