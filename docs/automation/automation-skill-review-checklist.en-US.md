# Automation Skill Review Checklist

Use this checklist when reviewing automation behavior, prompts, or generated plans.

## Mode Check

- What mode is the current turn in: create automation, owner task, or status/report?
- Did the behavior stay inside that mode?
- Did the model cross from report mode into repair mode without permission?

## Ownership Check

- If a role is meant to own the automation, did that role create it?
- Did `main_agent` overstep and create the automation directly?
- Is `owner` aligned with the intended execution identity?

## Owner Task Check

- Did the owner task execute the business action defined by `automation_goal`?
- Did the owner task avoid changing automation configuration?
- Did the owner task avoid changing watcher scripts or role configuration?

## Status/Report Check

- Did the status/report response stay read-only?
- Did it summarize current state clearly?
- Did it avoid sneaking in repairs or reconfiguration?

## Watcher Contract Check

- Is `watch_script` runnable now?
- Does the watcher output match the runtime's expected signal contract?
- Did the draft avoid fake shell loops or unmanaged background behavior?

## Routing And Policy Check

- Are `report_target` and `escalation_target` explicit where supervision matters?
- Are policy defaults normalized to the simple policy envelope?
- Is supervision routed to the intended parent instead of being trapped inside the same role by accident?
