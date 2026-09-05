import { run, Ui } from "../index";

run((u: Ui) => {
  u.dialog(" Hello vtui ", 40, () => {
    const name = u.edit("&Name:", "Type here...");
    if (u.button("&Ok")) {
      u.message(" Result ", `You typed:\n${name}`);
    }
  });
});
