# Contributing to Midaz

At Midaz, we believe in the power of collaboration and the incredible impact that each individual can make. Whether you're fixing a bug, proposing a new feature, improving documentation, or offering your unique perspective, your contributions are highly valued and play a crucial role in the evolution of Midaz.

#### Why Contribute?

* **Impact**: Your work will directly impact and improve a project used by organizations around the world, making their operations smoother and more efficient.
* **Learn and Grow**: Contributing to Midaz offers you a unique opportunity to learn from a community of talented developers, enhancing your skills and knowledge in architecture design, CQRS, Ports & Adapters, and more.
* **Community**: Join a welcoming and supportive community of developers who share your passion for creating high-quality, open-source software.

#### How Can You Contribute?

* **Code Contributions**: From minor fixes to major features, your code contributions are always welcome. Our architecture and minimal dependencies make it easy for you to understand and enhance Midaz.
* **Documentation**: Help us improve our documentation to make Midaz more accessible and understandable for everyone.
* **Feedback and Ideas**: Share your insights, suggestions, and innovative ideas to help us shape the future of Midaz.
* **Testing**: Assist in testing new releases or features, providing valuable feedback to ensure stability and usability.

## Our Workflow

Our contribution process is straightforward:

```
[Issue] > Pull request > Commit Signing > Code Review > Merge
```

For most changes, we ask that you first create an issue to discuss your proposed changes. This helps us to track the conversation and feedback. However, for minor edits like typos, you can directly submit a pull request.

## Commit Message Guidelines

We adopt the [Conventional Commit](https://www.conventionalcommits.org/en/v1.0.0/) format to ensure our commit history is readable and easy to follow. This format is part of a broader set of guidelines designed to facilitate automatic versioning and changelog generation:

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

---

<br />
The commit contains the following structural elements, to communicate intent to the
consumers of your library:

<br />

1. **fix:** a commit of the _type_ `fix` patches a bug in your codebase (this correlates with [`PATCH`](http://semver.org/#summary) in Semantic Versioning).
2. **feat:** a commit of the _type_ `feat` introduces a new feature to the codebase (this correlates with [`MINOR`](http://semver.org/#summary) in Semantic Versioning).
3. **BREAKING CHANGE:** a commit that has a footer `BREAKING CHANGE:`, or appends a `!` after the type/scope, introduces a breaking API change (correlating with [`MAJOR`](http://semver.org/#summary) in Semantic Versioning).
   A BREAKING CHANGE can be part of commits of any _type_.
4. _types_ other than `fix:` and `feat:` are allowed, for example [@commitlint/config-conventional](https://github.com/conventional-changelog/commitlint/tree/master/%40commitlint/config-conventional) (based on the [the Angular convention](https://github.com/angular/angular/blob/22b96b9/CONTRIBUTING.md#-commit-message-guidelines)) recommends `build:`, `chore:`,
   `ci:`, `docs:`, `style:`, `refactor:`, `perf:`, `test:`, and others.
5. _footers_ other than `BREAKING CHANGE: <description>` may be provided and follow a convention similar to
   [git trailer format](https://git-scm.com/docs/git-interpret-trailers).

Additional types are not mandated by the Conventional Commits specification, and have no implicit effect in Semantic Versioning (unless they include a BREAKING CHANGE).
`<br /><br />`
A scope may be provided to a commit's type, to provide additional contextual information and is contained within parenthesis, e.g., `feat(parser): add ability to parse arrays`.

## How to Submit a Pull Request

#### Commit Signing Requirement

For the integrity and verification of contributions, we require that all commits be signed, adhering to the [developercertificate.org](https://developercertificate.org/). This certifies that you have the rights to submit the work under our project's license and that you agree to the DCO statement:

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

Then you just add a line to every git commit message:

Signed-off-by: Joe Smith <joe.smith@example.com>
Use your real name (sorry, no pseudonyms or anonymous contributions.)

If you set your user.name and user.email git configs, you can sign your commit automatically with git commit -s.

By following these guidelines, you help ensure Midaz is a welcoming, efficient, and valuable project for everyone. Thank you for your contributions and for being a part of our community!

Before sending us a pull request, please ensure that,

- Fork the midaz repo on GitHub, clone it on your machine.
- Create a feature or fix branch with your changes.
- You are working against the latest source on the `main` branch.
- Modify the source; please focus only on the specific change you are contributing.
- Ensure local tests pass.
- Commit to your fork using clear commit messages.
- Send us a pull request, answering any default questions in the pull request interface.
- Pay attention to any automated CI failures reported in the pull request, and stay involved in the conversation
- Once you've pushed your commits to GitHub, make sure that your branch can be auto-merged (there are no merge conflicts). If not, on your computer, merge main into your branch, resolve any merge conflicts, make sure everything still runs correctly and passes all the tests, and then push up those changes.
- Once the change has been approved and merged, we will inform you in a comment.